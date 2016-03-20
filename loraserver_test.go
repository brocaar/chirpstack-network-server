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

	Convey("Given a dummy gateway and application backend and a clean Redis database", t, func() {
		app := &testApplicationBackend{
			rxPacketChan: make(chan ApplicationRXPacket, 1),
		}
		gw := &testGatewayBackend{
			rxPacketChan: make(chan RXPacket),
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
				FCntDown: 0,

				AppEUI: lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1},
			}
			So(saveNodeSession(p, ns), ShouldBeNil)

			Convey("Given UnconfirmedDataUp packet", func() {
				macPL := lorawan.NewMACPayload(true)
				macPL.FHDR = lorawan.FHDR{
					DevAddr: ns.DevAddr,
					FCnt:    10,
				}
				macPL.FPort = 1
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
					RXInfo: RXInfo{
						MAC:        [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
						Time:       time.Now().UTC(),
						Timestamp:  708016819,
						Frequency:  868.5,
						Channel:    2,
						RFChain:    1,
						CRCStatus:  1,
						Modulation: "LORA",
						DataRate:   DataRate{LoRa: "SF7BW125"},
						CodeRate:   "4/5",
						RSSI:       -51,
						LoRaSNR:    7,
						Size:       16,
					},
					PHYPayload: phy,
				}

				Convey("When calling handleRXPacket", func() {
					So(handleRXPacket(ctx, rxPacket), ShouldBeNil)

					Convey("Then the packet is correctly received by the application backend", func() {
						packet := <-app.rxPacketChan
						So(packet, ShouldResemble, ApplicationRXPacket{
							MType:        lorawan.UnconfirmedDataUp,
							DevEUI:       ns.DevEUI,
							ACK:          false,
							FPort:        1,
							GatewayCount: 1,
							Data:         []byte("hello!"),
						})
					})

					Convey("Then the FCntUp is packet FCnt + 1", func() {
						nsUpdated, err := getNodeSession(p, ns.DevAddr)
						So(err, ShouldBeNil)
						So(nsUpdated.FCntUp, ShouldEqual, 11)
					})
				})

				Convey("Given that the application backend returns an error", func() {
					app.err = errors.New("BOOM")

					Convey("When calling handleRXPacket", func() {
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
			})
		})
	})
}

func TestHandleJoinRequestPackets(t *testing.T) {
	conf := getConfig()

	Convey("Given a dummy gateway and application backend and a clean Postgres and Redis database", t, func() {
		a := &testApplicationBackend{
			rxPacketChan: make(chan ApplicationRXPacket, 1),
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
					RXInfo: RXInfo{
						MAC:        [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
						Time:       time.Now().UTC(),
						Timestamp:  708016819,
						Frequency:  868.5,
						Channel:    2,
						RFChain:    1,
						CRCStatus:  1,
						Modulation: "LORA",
						DataRate:   DataRate{LoRa: "SF7BW125"},
						CodeRate:   "4/5",
						RSSI:       -51,
						LoRaSNR:    7,
						Size:       16,
					},
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
