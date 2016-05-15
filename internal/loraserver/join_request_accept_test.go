package loraserver

import (
	"testing"
	"time"

	"github.com/brocaar/loraserver/models"
	"github.com/brocaar/lorawan"
	. "github.com/smartystreets/goconvey/convey"
)

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
