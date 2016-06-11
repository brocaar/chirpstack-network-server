package loraserver

import (
	"errors"
	"testing"
	"time"

	"github.com/brocaar/loraserver/models"
	"github.com/brocaar/lorawan"
	. "github.com/smartystreets/goconvey/convey"
)

func TestHandleDataUpPackets(t *testing.T) {
	conf := getConfig()

	Convey("Given a clean state", t, func() {
		app := &testApplicationBackend{
			rxPayloadChan:           make(chan models.RXPayload, 1),
			txPayloadChan:           make(chan models.TXPayload, 1),
			notificationPayloadChan: make(chan interface{}, 10),
		}
		gw := &testGatewayBackend{
			rxPacketChan: make(chan models.RXPacket, 1),
			txPacketChan: make(chan models.TXPacket, 1),
		}
		ctrl := &testControllerBackend{
			rxMACPayloadChan:  make(chan models.MACPayload, 2),
			txMACPayloadChan:  make(chan models.MACPayload, 1),
			rxInfoPayloadChan: make(chan models.RXInfoPayload, 1),
		}
		p := NewRedisPool(conf.RedisURL)
		mustFlushRedis(p)

		ctx := Context{
			RedisPool:   p,
			Gateway:     gw,
			Application: app,
			Controller:  ctrl,
		}

		Convey("Given a stored node session", func() {
			ns := models.NodeSession{
				DevAddr:  [4]byte{1, 2, 3, 4},
				DevEUI:   [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
				AppSKey:  [16]byte{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
				NwkSKey:  [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
				FCntUp:   8,
				FCntDown: 5,
				AppEUI:   [8]byte{8, 7, 6, 5, 4, 3, 2, 1},
			}
			So(saveNodeSession(p, ns), ShouldBeNil)

			Convey("Given two mac commands", func() {
				mac1 := lorawan.MACCommand{
					CID: lorawan.LinkCheckReq,
				}
				mac2 := lorawan.MACCommand{
					CID: lorawan.LinkADRAns,
					Payload: &lorawan.LinkADRAnsPayload{
						ChannelMaskACK: true,
						DataRateACK:    false,
						PowerACK:       false,
					},
				}

				Convey("Given an UnconfirmedDataUp packet with the mac commands as FOpts", func() {
					phy := lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ns.DevAddr,
								FCnt:    10,
								FOpts:   []lorawan.MACCommand{mac1, mac2},
							},
						},
					}

					So(phy.EncryptFRMPayload(ns.NwkSKey), ShouldBeNil)
					So(phy.SetMIC(ns.NwkSKey), ShouldBeNil)
					rxPacket := models.RXPacket{
						PHYPayload: phy,
						RXInfo: models.RXInfo{
							Frequency: Band.UplinkChannels[0].Frequency,
							DataRate:  Band.DataRates[Band.UplinkChannels[0].DataRates[0]],
						},
					}

					Convey("When calling handleRXPacket", func() {
						So(handleRXPacket(ctx, rxPacket), ShouldBeNil)

						Convey("Then an rx info payload was sent to the network-controller", func() {
							_ = <-ctrl.rxInfoPayloadChan
						})

						Convey("Then the MAC commands were received by the network-controller", func() {
							pl1 := <-ctrl.rxMACPayloadChan
							b1, err := mac1.MarshalBinary()
							So(err, ShouldBeNil)
							pl2 := <-ctrl.rxMACPayloadChan
							b2, err := mac2.MarshalBinary()
							So(err, ShouldBeNil)

							So(pl1, ShouldResemble, models.MACPayload{
								DevEUI:     ns.DevEUI,
								MACCommand: b1,
							})
							So(pl2, ShouldResemble, models.MACPayload{
								DevEUI:     ns.DevEUI,
								MACCommand: b2,
							})
						})
					})
				})

				Convey("Given an UnconfirmedDataUp packet with the mac commands as FRMPayload", func() {
					fPort := uint8(0)
					phy := lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: ns.DevAddr,
								FCnt:    10,
							},
							FPort:      &fPort,
							FRMPayload: []lorawan.Payload{&mac1, &mac2},
						},
					}

					So(phy.EncryptFRMPayload(ns.NwkSKey), ShouldBeNil)
					So(phy.SetMIC(ns.NwkSKey), ShouldBeNil)

					rxPacket := models.RXPacket{
						PHYPayload: phy,
						RXInfo: models.RXInfo{
							Frequency: Band.UplinkChannels[0].Frequency,
							DataRate:  Band.DataRates[Band.UplinkChannels[0].DataRates[0]],
						},
					}

					Convey("When calling handleRXPacket", func() {
						So(handleRXPacket(ctx, rxPacket), ShouldBeNil)

						Convey("Then an rx info payload was sent to the network-controller", func() {
							_ = <-ctrl.rxInfoPayloadChan
						})

						Convey("Then the MAC commands were received by the network-controller", func() {
							pl1 := <-ctrl.rxMACPayloadChan
							b1, err := mac1.MarshalBinary()
							So(err, ShouldBeNil)
							pl2 := <-ctrl.rxMACPayloadChan
							b2, err := mac2.MarshalBinary()
							So(err, ShouldBeNil)

							So(pl1, ShouldResemble, models.MACPayload{
								DevEUI:     ns.DevEUI,
								MACCommand: b1,
								FRMPayload: true,
							})
							So(pl2, ShouldResemble, models.MACPayload{
								DevEUI:     ns.DevEUI,
								MACCommand: b2,
								FRMPayload: true,
							})
						})
					})
				})
			})

			Convey("Given an UnconfirmedDataUp packet", func() {
				fPort := uint8(1)
				phy := lorawan.PHYPayload{
					MHDR: lorawan.MHDR{
						MType: lorawan.UnconfirmedDataUp,
						Major: lorawan.LoRaWANR1,
					},
					MACPayload: &lorawan.MACPayload{
						FHDR: lorawan.FHDR{
							DevAddr: ns.DevAddr,
							FCnt:    10,
						},
						FPort: &fPort,
						FRMPayload: []lorawan.Payload{
							&lorawan.DataPayload{Bytes: []byte("hello!")},
						},
					},
				}

				So(phy.EncryptFRMPayload(ns.AppSKey), ShouldBeNil)
				So(phy.SetMIC(ns.NwkSKey), ShouldBeNil)

				rxPacket := models.RXPacket{
					PHYPayload: phy,
					RXInfo: models.RXInfo{
						Frequency: Band.UplinkChannels[0].Frequency,
						DataRate:  Band.DataRates[Band.UplinkChannels[0].DataRates[0]],
					},
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

					Convey("Then a rx info payload was sent to the network-controller", func() {
						_ = <-ctrl.rxInfoPayloadChan
					})

					Convey("Then the packet is correctly received by the application backend", func() {
						packet := <-app.rxPayloadChan
						So(packet, ShouldResemble, models.RXPayload{
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
					txPayload := models.TXPayload{
						Confirmed: false,
						DevEUI:    ns.DevEUI,
						FPort:     5,
						Data:      []byte("hello back!"),
					}
					So(addTXPayloadToQueue(p, txPayload), ShouldBeNil)

					Convey("When calling handleRXPacket", func() {
						So(handleRXPacket(ctx, rxPacket), ShouldBeNil)

						Convey("Then a rx info payload was sent to the network-controller", func() {
							_ = ctrl.rxInfoPayloadChan
						})

						Convey("Then the packet is received by the application backend", func() {
							_ = <-app.rxPayloadChan
						})

						Convey("Then a packet is sent to the gateway", func() {
							txPacket := <-gw.txPacketChan

							macPL, ok := txPacket.PHYPayload.MACPayload.(*lorawan.MACPayload)
							So(ok, ShouldBeTrue)

							Convey("Then this packet contains the expected values", func() {
								So(txPacket.PHYPayload.MHDR.MType, ShouldEqual, lorawan.UnconfirmedDataDown)
								So(macPL.FHDR.FCnt, ShouldEqual, ns.FCntDown)
								So(macPL.FHDR.FCtrl.ACK, ShouldBeFalse)
								So(*macPL.FPort, ShouldEqual, 5)

								So(txPacket.PHYPayload.DecryptFRMPayload(ns.AppSKey), ShouldBeNil)
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
							txPayload, _, err := getTXPayloadAndRemainingFromQueue(p, ns.DevEUI)
							So(err, ShouldBeNil)
							So(txPayload, ShouldBeNil)
						})
					})
				})

				Convey("Given an enqueued TXPayload which exceeds the max size", func() {
					txPayload := models.TXPayload{
						DevEUI:    ns.DevEUI,
						FPort:     5,
						Data:      []byte("hello back!hello back!hello back!hello back!hello back!hello back!hello back!hello back!hello back!hello back!"),
						Reference: "testcase",
					}
					So(addTXPayloadToQueue(p, txPayload), ShouldBeNil)

					Convey("When calling handleRXPacket", func() {
						So(handleRXPacket(ctx, rxPacket), ShouldBeNil)

						Convey("Then a rx info payload and error notification were sent", func() {
							_ = ctrl.rxInfoPayloadChan
							notification := <-app.notificationPayloadChan
							So(notification, ShouldHaveSameTypeAs, models.ErrorPayload{})
						})

						Convey("Then the TXPayload queue is empty", func() {
							txPayload, _, err := getTXPayloadAndRemainingFromQueue(p, ns.DevEUI)
							So(err, ShouldBeNil)
							So(txPayload, ShouldBeNil)
						})
					})
				})

				Convey("Given an enqueued TXPayload (confirmed)", func() {
					txPayload := models.TXPayload{
						Confirmed: true,
						DevEUI:    ns.DevEUI,
						FPort:     5,
						Data:      []byte("hello back!"),
					}
					So(addTXPayloadToQueue(p, txPayload), ShouldBeNil)

					Convey("When calling handleRXPacket", func() {
						So(handleRXPacket(ctx, rxPacket), ShouldBeNil)

						Convey("Then an rx info payload was sent to the network-controller", func() {
							_ = ctrl.rxInfoPayloadChan
						})

						Convey("Then the packet is received by the application backend", func() {
							_ = <-app.rxPayloadChan
						})

						Convey("Then a packet is sent to the gateway", func() {
							txPacket := <-gw.txPacketChan

							macPL, ok := txPacket.PHYPayload.MACPayload.(*lorawan.MACPayload)
							So(ok, ShouldBeTrue)

							Convey("Then this packet contains the expected values", func() {
								So(txPacket.PHYPayload.MHDR.MType, ShouldEqual, lorawan.ConfirmedDataDown)
								So(macPL.FHDR.FCnt, ShouldEqual, ns.FCntDown)
								So(macPL.FHDR.FCtrl.ACK, ShouldBeFalse)
								So(*macPL.FPort, ShouldEqual, 5)

								So(txPacket.PHYPayload.DecryptFRMPayload(ns.AppSKey), ShouldBeNil)
								So(len(macPL.FRMPayload), ShouldEqual, 1)
								pl, ok := macPL.FRMPayload[0].(*lorawan.DataPayload)
								So(ok, ShouldBeTrue)
								So(pl.Bytes, ShouldResemble, []byte("hello back!"))
							})
						})

						Convey("Then the TXPayload is still in the queue", func() {
							tx, _, err := getTXPayloadAndRemainingFromQueue(p, ns.DevEUI)
							So(err, ShouldBeNil)
							So(tx, ShouldResemble, &txPayload)
						})

						Convey("Then the FCntDown was not incremented", func() {
							ns2, err := getNodeSession(p, ns.DevAddr)
							So(err, ShouldBeNil)
							So(ns2.FCntDown, ShouldEqual, ns.FCntDown)

							Convey("Given the node sends an ACK", func() {
								phy := lorawan.PHYPayload{
									MHDR: lorawan.MHDR{
										MType: lorawan.UnconfirmedDataUp,
										Major: lorawan.LoRaWANR1,
									},
									MACPayload: &lorawan.MACPayload{
										FHDR: lorawan.FHDR{
											DevAddr: ns.DevAddr,
											FCnt:    11,
											FCtrl: lorawan.FCtrl{
												ACK: true,
											},
										},
									},
								}
								So(phy.SetMIC(ns.NwkSKey), ShouldBeNil)

								rxPacket := models.RXPacket{
									PHYPayload: phy,
								}
								_ = <-ctrl.rxInfoPayloadChan // empty the channel
								So(handleRXPacket(ctx, rxPacket), ShouldBeNil)

								Convey("Then the FCntDown was incremented", func() {
									ns2, err := getNodeSession(p, ns.DevAddr)
									So(err, ShouldBeNil)
									So(ns2.FCntDown, ShouldEqual, ns.FCntDown+1)

									Convey("Then the TXPayload queue is empty", func() {
										txPayload, _, err := getTXPayloadAndRemainingFromQueue(p, ns.DevEUI)
										So(err, ShouldBeNil)
										So(txPayload, ShouldBeNil)
									})

									Convey("Then an rx info payload and ACK notification were sent", func() {
										_ = ctrl.rxInfoPayloadChan
										notification := <-app.notificationPayloadChan
										So(notification, ShouldHaveSameTypeAs, models.ACKNotification{})
									})
								})
							})
						})
					})
				})
			})

			Convey("Given a ConfirmedDataUp packet", func() {
				fPort := uint8(1)
				phy := lorawan.PHYPayload{
					MHDR: lorawan.MHDR{
						MType: lorawan.ConfirmedDataUp,
						Major: lorawan.LoRaWANR1,
					},
					MACPayload: &lorawan.MACPayload{
						FHDR: lorawan.FHDR{
							DevAddr: ns.DevAddr,
							FCnt:    10,
						},
						FPort: &fPort,
						FRMPayload: []lorawan.Payload{
							&lorawan.DataPayload{Bytes: []byte("hello!")},
						},
					},
				}

				So(phy.EncryptFRMPayload(ns.AppSKey), ShouldBeNil)
				So(phy.SetMIC(ns.NwkSKey), ShouldBeNil)

				rxPacket := models.RXPacket{
					PHYPayload: phy,
					RXInfo: models.RXInfo{
						Frequency: Band.UplinkChannels[0].Frequency,
						DataRate:  Band.DataRates[Band.UplinkChannels[0].DataRates[0]],
					},
				}

				Convey("Given an enqueued TXPayload which exceeds the max size", func() {
					txPayload := models.TXPayload{
						DevEUI:    ns.DevEUI,
						FPort:     5,
						Data:      []byte("hello back!hello back!hello back!hello back!hello back!hello back!hello back!hello back!hello back!hello back!"),
						Reference: "testcase",
					}
					So(addTXPayloadToQueue(p, txPayload), ShouldBeNil)

					Convey("When calling handleRXPacket", func() {
						So(handleRXPacket(ctx, rxPacket), ShouldBeNil)

						Convey("Then an rx info payload and error notification were sent", func() {
							_ = ctrl.rxInfoPayloadChan
							notification := <-app.notificationPayloadChan
							So(notification, ShouldHaveSameTypeAs, models.ErrorPayload{})
						})

						Convey("Then the TXPayload queue is empty", func() {
							txPayload, _, err := getTXPayloadAndRemainingFromQueue(p, ns.DevEUI)
							So(err, ShouldBeNil)
							So(txPayload, ShouldBeNil)
						})
					})
				})

				Convey("When calling handleRXPacket", func() {
					So(handleRXPacket(ctx, rxPacket), ShouldBeNil)

					Convey("Then the packet is correctly received by the application backend", func() {
						packet := <-app.rxPayloadChan
						So(packet, ShouldResemble, models.RXPayload{
							DevEUI:       ns.DevEUI,
							FPort:        1,
							GatewayCount: 1,
							Data:         []byte("hello!"),
						})
					})

					Convey("Then a ACK packet is sent to the gateway", func() {
						txPacket := <-gw.txPacketChan

						macPL, ok := txPacket.PHYPayload.MACPayload.(*lorawan.MACPayload)
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
			rxPayloadChan:           make(chan models.RXPayload, 1),
			notificationPayloadChan: make(chan interface{}, 10),
		}
		g := &testGatewayBackend{
			rxPacketChan: make(chan models.RXPacket, 1),
			txPacketChan: make(chan models.TXPacket, 1),
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
			app := models.Application{
				AppEUI: [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
				Name:   "test app",
			}
			So(createApplication(ctx.DB, app), ShouldBeNil)

			node := models.Node{
				DevEUI: [8]byte{8, 7, 6, 5, 4, 3, 2, 1},
				AppEUI: app.AppEUI,
				AppKey: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},

				RXDelay:     3,
				RX1DROffset: 2,
			}
			So(createNode(ctx.DB, node), ShouldBeNil)

			Convey("Given a JoinRequest packet", func() {
				phy := lorawan.PHYPayload{
					MHDR: lorawan.MHDR{
						MType: lorawan.JoinRequest,
						Major: lorawan.LoRaWANR1,
					},
					MACPayload: &lorawan.JoinRequestPayload{
						AppEUI:   app.AppEUI,
						DevEUI:   node.DevEUI,
						DevNonce: [2]byte{1, 2},
					},
				}
				So(phy.SetMIC(node.AppKey), ShouldBeNil)

				rxPacket := models.RXPacket{
					PHYPayload: phy,
					RXInfo: models.RXInfo{
						Frequency: Band.UplinkChannels[0].Frequency,
						DataRate:  Band.DataRates[Band.UplinkChannels[0].DataRates[0]],
					},
				}

				Convey("When calling handleRXPacket", func() {
					So(handleRXPacket(ctx, rxPacket), ShouldBeNil)

					Convey("Then a JoinAccept was sent to the node", func() {
						txPacket := <-g.txPacketChan
						phy := txPacket.PHYPayload
						So(phy.DecryptJoinAcceptPayload(node.AppKey), ShouldBeNil)
						So(phy.MHDR.MType, ShouldEqual, lorawan.JoinAccept)

						Convey("Then it was sent after 5s", func() {
							So(txPacket.TXInfo.Timestamp, ShouldEqual, rxPacket.RXInfo.Timestamp+uint32(5*time.Second/time.Microsecond))
						})

						Convey("Then the RXDelay is set to 3s", func() {
							jaPL := phy.MACPayload.(*lorawan.JoinAcceptPayload)
							So(jaPL.RXDelay, ShouldEqual, 3)
						})

						Convey("Then the DLSettings are set correctly", func() {
							jaPL := phy.MACPayload.(*lorawan.JoinAcceptPayload)
							So(jaPL.DLSettings.RX2DataRate, ShouldEqual, uint8(Band.RX2DataRate))
							So(jaPL.DLSettings.RX1DROffset, ShouldEqual, node.RX1DROffset)
						})

						Convey("Then a node-session was created", func() {
							jaPL := phy.MACPayload.(*lorawan.JoinAcceptPayload)

							_, err := getNodeSession(ctx.RedisPool, jaPL.DevAddr)
							So(err, ShouldBeNil)
						})

						Convey("Then the dev-nonce was added to the used dev-nonces", func() {
							node, err := getNode(ctx.DB, node.DevEUI)
							So(err, ShouldBeNil)
							So([2]byte{1, 2}, ShouldBeIn, node.UsedDevNonces)
						})

						Convey("Then a join notification was sent to the application", func() {
							notification := <-a.notificationPayloadChan
							join, ok := notification.(models.JoinNotification)
							So(ok, ShouldBeTrue)
							So(join.DevEUI, ShouldResemble, node.DevEUI)
						})
					})
				})
			})
		})
	})
}
