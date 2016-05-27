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
		p := NewRedisPool(conf.RedisURL)
		mustFlushRedis(p)

		ctx := Context{
			RedisPool:   p,
			Gateway:     gw,
			Application: app,
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

					Convey("Then a rx info notification was sent", func() {
						notification := <-app.notificationPayloadChan
						_, ok := notification.(models.RXInfoPayload)
						So(ok, ShouldBeTrue)
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

						Convey("Then a rx info notification was sent", func() {
							notification := <-app.notificationPayloadChan
							So(notification, ShouldHaveSameTypeAs, models.RXInfoPayload{})
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

						Convey("Then an rx info and error notification was sent", func() {
							notification := <-app.notificationPayloadChan
							So(notification, ShouldHaveSameTypeAs, models.RXInfoPayload{})
							notification = <-app.notificationPayloadChan
							So(notification, ShouldHaveSameTypeAs, models.ErrorNotification{})
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

						Convey("Then an rx info and ACK notification were sent", func() {
							notification := <-app.notificationPayloadChan
							So(notification, ShouldHaveSameTypeAs, models.RXInfoPayload{})
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

								_ = <-app.notificationPayloadChan // drain the notification channel
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

									Convey("Then an rx info and ACK notification were sent", func() {
										notification := <-app.notificationPayloadChan
										So(notification, ShouldHaveSameTypeAs, models.RXInfoPayload{})
										notification = <-app.notificationPayloadChan
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

						Convey("Then an rx info and error notification were sent", func() {
							notification := <-app.notificationPayloadChan
							So(notification, ShouldHaveSameTypeAs, models.RXInfoPayload{})
							notification = <-app.notificationPayloadChan
							So(notification, ShouldHaveSameTypeAs, models.ErrorNotification{})
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
