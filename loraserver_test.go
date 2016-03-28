package loraserver

import (
	"errors"
	"testing"
	"time"

	"github.com/brocaar/lorawan"
	. "github.com/smartystreets/goconvey/convey"
)

func TestHandleDataUpPackets(t *testing.T) {
	conf := getConfig()

	Convey("Given a clean state", t, func() {
		app := &testApplicationBackend{
			rxPayloadChan: make(chan RXPayload, 1),
			txPayloadChan: make(chan TXPayload),
		}
		gw := &testGatewayBackend{
			rxPacketChan: make(chan RXPacket),
			txPacketChan: make(chan TXPacket, 2),
		}
		p := NewRedisPool(conf.RedisURL)
		mustFlushRedis(p)

		ctx := Context{
			RedisPool:   p,
			Gateway:     gw,
			Application: app,
		}

		Convey("Given a stored node session", func() {
			ns := NodeSession{
				DevAddr:  lorawan.DevAddr{1, 2, 3, 4},
				DevEUI:   lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
				AppSKey:  lorawan.AES128Key{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
				NwkSKey:  lorawan.AES128Key{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
				FCntUp:   8,
				FCntDown: 5,
				AppEUI:   lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1},
			}
			So(saveNodeSession(p, ns), ShouldBeNil)

			Convey("Given an UnconfirmedDataUp packet", func() {
				macPL := lorawan.NewMACPayload(true)
				macPL.FHDR = lorawan.FHDR{
					DevAddr: ns.DevAddr,
					FCnt:    10,
				}
				fPort := uint8(1)
				macPL.FPort = &fPort
				macPL.FRMPayload = []lorawan.Payload{
					&lorawan.DataPayload{Bytes: []byte("hello!")},
				}
				So(macPL.EncryptFRMPayload(ns.AppSKey), ShouldBeNil)

				phy := lorawan.NewPHYPayload(true)
				phy.MHDR = lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				}
				phy.MACPayload = macPL
				So(phy.SetMIC(ns.NwkSKey), ShouldBeNil)

				rxPacket := RXPacket{
					PHYPayload: phy,
				}

				Convey("Given that the application backend returns an error", func() {
					app.err = errors.New("BOOM")

					Convey("Then handleRXPacket returns an error", func() {
						So(handleRXPacket(ctx, rxPacket), ShouldNotBeNil)

						Convey("Then the FCntUp has not been incremented", func() {
							nsUpdated, err := getNodeSession(p, ns.DevAddr)
							So(err, ShouldBeNil)
							So(nsUpdated, ShouldResemble, ns)
						})
					})
				})

				Convey("When the frame-counter is invalid", func() {
					ns.FCntUp = 11
					So(saveNodeSession(p, ns), ShouldBeNil)

					Convey("Then handleRXPacket returns a frame-counter related error", func() {
						err := handleRXPacket(ctx, rxPacket)
						So(err, ShouldResemble, errors.New("invalid FCnt or too many dropped frames"))
					})
				})

				Convey("When the MIC is invalid", func() {
					rxPacket.PHYPayload.MIC = [4]byte{1, 2, 3, 4}

					Convey("Then handleRXPacket returns a MIC related error", func() {
						err := handleRXPacket(ctx, rxPacket)
						So(err, ShouldResemble, errors.New("invalid MIC"))
					})
				})

				Convey("When calling handleRXPacket", func() {
					So(handleRXPacket(ctx, rxPacket), ShouldBeNil)

					Convey("Then the packet is correctly received by the application backend", func() {
						packet := <-app.rxPayloadChan
						So(packet, ShouldResemble, RXPayload{
							DevEUI:       ns.DevEUI,
							FPort:        1,
							GatewayCount: 1,
							Data:         []byte("hello!"),
						})
					})

					Convey("Then the FCntUp must be synced", func() {
						nsUpdated, err := getNodeSession(p, ns.DevAddr)
						So(err, ShouldBeNil)
						So(nsUpdated.FCntUp, ShouldEqual, 10)
					})

					Convey("Then no downlink data was sent to the gateway", func() {
						var received bool
						select {
						case <-gw.txPacketChan:
							received = true
						case <-time.After(time.Second):
							// nothing to do
						}
						So(received, ShouldEqual, false)
					})
				})

				Convey("Given an enqueued TXPayload (unconfirmed)", func() {
					txPayload := TXPayload{
						Confirmed: false,
						DevEUI:    ns.DevEUI,
						FPort:     5,
						Data:      []byte("hello back!"),
					}
					So(addTXPayloadToQueue(p, txPayload), ShouldBeNil)

					Convey("When calling handleRXPacket", func() {
						So(handleRXPacket(ctx, rxPacket), ShouldBeNil)

						Convey("Then the packet is received by the application backend", func() {
							_ = <-app.rxPayloadChan
						})

						Convey("Then two identical packets are sent to the gateway (two receive windows)", func() {
							txPacket1 := <-gw.txPacketChan
							txPacket2 := <-gw.txPacketChan
							So(txPacket1.PHYPayload, ShouldResemble, txPacket2.PHYPayload)

							macPL, ok := txPacket1.PHYPayload.MACPayload.(*lorawan.MACPayload)
							So(ok, ShouldBeTrue)

							Convey("Then these packets contain the expected values", func() {
								So(txPacket1.PHYPayload.MHDR.MType, ShouldEqual, lorawan.UnconfirmedDataDown)
								So(macPL.FHDR.FCnt, ShouldEqual, ns.FCntDown)
								So(macPL.FHDR.FCtrl.ACK, ShouldBeFalse)
								So(*macPL.FPort, ShouldEqual, 5)

								So(macPL.DecryptFRMPayload(ns.AppSKey), ShouldBeNil)
								So(len(macPL.FRMPayload), ShouldEqual, 1)
								pl, ok := macPL.FRMPayload[0].(*lorawan.DataPayload)
								So(ok, ShouldBeTrue)
								So(pl.Bytes, ShouldResemble, []byte("hello back!"))
							})
						})

						Convey("Then the FCntDown was incremented", func() {
							ns2, err := getNodeSession(p, ns.DevAddr)
							So(err, ShouldBeNil)
							So(ns2.FCntDown, ShouldEqual, ns.FCntDown+1)
						})

						Convey("Then the TXPayload queue is empty", func() {
							_, _, err := getTXPayloadAndRemainingFromQueue(p, ns.DevEUI)
							So(err, ShouldEqual, errDoesNotExist)
						})
					})
				})

				Convey("Given an enqueued TXPayload (confirmed)", func() {
					txPayload := TXPayload{
						Confirmed: true,
						DevEUI:    ns.DevEUI,
						FPort:     5,
						Data:      []byte("hello back!"),
					}
					So(addTXPayloadToQueue(p, txPayload), ShouldBeNil)

					Convey("When calling handleRXPacket", func() {
						So(handleRXPacket(ctx, rxPacket), ShouldBeNil)

						Convey("Then the packet is received by the application backend", func() {
							_ = <-app.rxPayloadChan
						})

						Convey("Then two identical packets are sent to the gateway (two receive windows)", func() {
							txPacket1 := <-gw.txPacketChan
							txPacket2 := <-gw.txPacketChan
							So(txPacket1.PHYPayload, ShouldResemble, txPacket2.PHYPayload)

							macPL, ok := txPacket1.PHYPayload.MACPayload.(*lorawan.MACPayload)
							So(ok, ShouldBeTrue)

							Convey("Then these packets contain the expected values", func() {
								So(txPacket1.PHYPayload.MHDR.MType, ShouldEqual, lorawan.ConfirmedDataDown)
								So(macPL.FHDR.FCnt, ShouldEqual, ns.FCntDown)
								So(macPL.FHDR.FCtrl.ACK, ShouldBeFalse)
								So(*macPL.FPort, ShouldEqual, 5)

								So(macPL.DecryptFRMPayload(ns.AppSKey), ShouldBeNil)
								So(len(macPL.FRMPayload), ShouldEqual, 1)
								pl, ok := macPL.FRMPayload[0].(*lorawan.DataPayload)
								So(ok, ShouldBeTrue)
								So(pl.Bytes, ShouldResemble, []byte("hello back!"))
							})
						})

						Convey("Then the TXPayload is still in the queue", func() {
							tx, _, err := getTXPayloadAndRemainingFromQueue(p, ns.DevEUI)
							So(err, ShouldBeNil)
							So(tx, ShouldResemble, txPayload)
						})

						Convey("Then the FCntDown was not incremented", func() {
							ns2, err := getNodeSession(p, ns.DevAddr)
							So(err, ShouldBeNil)
							So(ns2.FCntDown, ShouldEqual, ns.FCntDown)

							Convey("Given the node sends an ACK", func() {
								macPL := lorawan.NewMACPayload(true)
								macPL.FHDR = lorawan.FHDR{
									DevAddr: ns.DevAddr,
									FCnt:    11,
									FCtrl: lorawan.FCtrl{
										ACK: true,
									},
								}
								phy := lorawan.NewPHYPayload(true)
								phy.MHDR = lorawan.MHDR{
									MType: lorawan.UnconfirmedDataUp,
									Major: lorawan.LoRaWANR1,
								}
								phy.MACPayload = macPL
								So(phy.SetMIC(ns.NwkSKey), ShouldBeNil)

								rxPacket := RXPacket{
									PHYPayload: phy,
								}

								So(handleRXPacket(ctx, rxPacket), ShouldBeNil)

								Convey("Then the FCntDown was incremented", func() {
									ns2, err := getNodeSession(p, ns.DevAddr)
									So(err, ShouldBeNil)
									So(ns2.FCntDown, ShouldEqual, ns.FCntDown+1)

									Convey("Then the TXPayload queue is empty", func() {
										_, _, err := getTXPayloadAndRemainingFromQueue(p, ns.DevEUI)
										So(err, ShouldEqual, errDoesNotExist)
									})
								})
							})
						})
					})
				})
			})

			Convey("Given a ConfirmedDataUp packet", func() {
				macPL := lorawan.NewMACPayload(true)
				macPL.FHDR = lorawan.FHDR{
					DevAddr: ns.DevAddr,
					FCnt:    10,
				}
				fPort := uint8(1)
				macPL.FPort = &fPort
				macPL.FRMPayload = []lorawan.Payload{
					&lorawan.DataPayload{Bytes: []byte("hello!")},
				}
				So(macPL.EncryptFRMPayload(ns.AppSKey), ShouldBeNil)

				phy := lorawan.NewPHYPayload(true)
				phy.MHDR = lorawan.MHDR{
					MType: lorawan.ConfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				}
				phy.MACPayload = macPL
				So(phy.SetMIC(ns.NwkSKey), ShouldBeNil)

				rxPacket := RXPacket{
					PHYPayload: phy,
				}

				Convey("When calling handleRXPacket", func() {
					So(handleRXPacket(ctx, rxPacket), ShouldBeNil)

					Convey("Then the packet is correctly received by the application backend", func() {
						packet := <-app.rxPayloadChan
						So(packet, ShouldResemble, RXPayload{
							DevEUI:       ns.DevEUI,
							FPort:        1,
							GatewayCount: 1,
							Data:         []byte("hello!"),
						})
					})

					Convey("Then two ACK packets are sent to the gateway (two receive windows)", func() {
						txPacket1 := <-gw.txPacketChan
						txPacket2 := <-gw.txPacketChan

						So(txPacket1.PHYPayload.MIC, ShouldEqual, txPacket2.PHYPayload.MIC)

						macPL, ok := txPacket1.PHYPayload.MACPayload.(*lorawan.MACPayload)
						So(ok, ShouldBeTrue)

						So(macPL.FHDR.FCtrl.ACK, ShouldBeTrue)
						So(macPL.FHDR.FCnt, ShouldEqual, 5)

						Convey("Then the FCntDown counter has incremented", func() {
							ns, err := getNodeSession(ctx.RedisPool, ns.DevAddr)
							So(err, ShouldBeNil)
							So(ns.FCntDown, ShouldEqual, 6)
						})
					})
				})

			})
		})
	})
}

func TestHandleJoinRequestPackets(t *testing.T) {
	conf := getConfig()

	Convey("Given a dummy gateway and application backend and a clean Postgres and Redis database", t, func() {
		a := &testApplicationBackend{
			rxPayloadChan: make(chan RXPayload, 1),
		}
		g := &testGatewayBackend{
			rxPacketChan: make(chan RXPacket),
			txPacketChan: make(chan TXPacket, 2),
		}
		p := NewRedisPool(conf.RedisURL)
		mustFlushRedis(p)
		db, err := OpenDatabase(conf.PostgresDSN)
		So(err, ShouldBeNil)
		mustResetDB(db)

		ctx := Context{
			RedisPool:   p,
			Gateway:     g,
			Application: a,
			DB:          db,
		}

		Convey("Given a node and application in the database", func() {
			app := Application{
				AppEUI: [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
				Name:   "test app",
			}
			So(createApplication(ctx.DB, app), ShouldBeNil)

			node := Node{
				DevEUI: [8]byte{8, 7, 6, 5, 4, 3, 2, 1},
				AppEUI: app.AppEUI,
				AppKey: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			}
			So(createNode(ctx.DB, node), ShouldBeNil)

			Convey("Given a JoinRequest packet", func() {
				phy := lorawan.NewPHYPayload(true)
				phy.MHDR = lorawan.MHDR{
					MType: lorawan.JoinRequest,
					Major: lorawan.LoRaWANR1,
				}
				phy.MACPayload = &lorawan.JoinRequestPayload{
					AppEUI:   app.AppEUI,
					DevEUI:   node.DevEUI,
					DevNonce: [2]byte{1, 2},
				}
				So(phy.SetMIC(node.AppKey), ShouldBeNil)

				rxPacket := RXPacket{
					PHYPayload: phy,
				}

				Convey("When calling handleRXPacket", func() {
					So(handleRXPacket(ctx, rxPacket), ShouldBeNil)

					Convey("Then a JoinAccept was sent to the node", func() {
						txPacket := <-g.txPacketChan
						phy := txPacket.PHYPayload
						So(phy.DecryptJoinAcceptPayload(node.AppKey), ShouldBeNil)
						So(phy.MHDR.MType, ShouldEqual, lorawan.JoinAccept)

						Convey("Then the first delay is 5 sec", func() {
							So(txPacket.TXInfo.Timestamp, ShouldEqual, rxPacket.RXInfo.Timestamp+uint32(5*time.Second/time.Microsecond))
						})

						Convey("Then the second delay is 6 sec", func() {
							txPacket = <-g.txPacketChan
							So(txPacket.TXInfo.Timestamp, ShouldEqual, rxPacket.RXInfo.Timestamp+uint32(6*time.Second/time.Microsecond))
						})

						Convey("Then a node-session was created", func() {
							jaPL, ok := phy.MACPayload.(*lorawan.JoinAcceptPayload)
							So(ok, ShouldBeTrue)

							_, err := getNodeSession(ctx.RedisPool, jaPL.DevAddr)
							So(err, ShouldBeNil)
						})

						Convey("Then the dev-nonce was added to the used dev-nonces", func() {
							node, err := getNode(ctx.DB, node.DevEUI)
							So(err, ShouldBeNil)
							So([2]byte{1, 2}, ShouldBeIn, node.UsedDevNonces)
						})
					})
				})
			})
		})
	})
}
