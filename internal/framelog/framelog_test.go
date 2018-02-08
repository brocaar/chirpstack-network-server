package framelog

import (
	"context"
	"log"
	"testing"
	"time"

	"github.com/brocaar/lorawan"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/test"
	. "github.com/smartystreets/goconvey/convey"
)

func TestFrameLog(t *testing.T) {
	conf := test.GetConfig()
	p := common.NewRedisPool(conf.RedisURL)
	common.RedisPool = p

	mac := lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}
	devEUI := lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1}

	Convey("Given a clean Redis database", t, func() {
		test.MustFlushRedis(common.RedisPool)

		Convey("Testing GetFrameLogForGateway", func() {
			logChannel := make(chan FrameLog, 1)
			ctx := context.Background()
			cctx, cancel := context.WithCancel(ctx)
			defer cancel()

			go func() {
				if err := GetFrameLogForGateway(cctx, mac, logChannel); err != nil {
					log.Fatal(err)
				}
			}()

			// some time to subscribe
			time.Sleep(time.Millisecond * 100)

			Convey("When calling LogUplinkFrameForGateways", func() {
				rxPacket := models.RXPacket{
					RXInfoSet: []models.RXInfo{
						{
							MAC: mac,
						},
					},
				}

				So(LogUplinkFrameForGateways(rxPacket), ShouldBeNil)

				Convey("Then the frame has been logged", func() {
					So(<-logChannel, ShouldResemble, FrameLog{
						UplinkFrame: &UplinkFrameLog{
							RXInfoSet: []models.RXInfo{
								{
									MAC: mac,
								},
							},
						},
					})
				})
			})

			Convey("When calling LogDownlinkFrameForGateway", func() {
				frameLog := DownlinkFrameLog{
					TXInfo: gw.TXInfo{
						MAC: mac,
					},
				}
				So(LogDownlinkFrameForGateway(frameLog), ShouldBeNil)

				Convey("Then the frame has been logged", func() {
					So(<-logChannel, ShouldResemble, FrameLog{
						DownlinkFrame: &frameLog,
					})
				})
			})
		})

		Convey("Testing GetFrameLogForDevice", func() {
			logChannel := make(chan FrameLog, 1)
			ctx := context.Background()
			cctx, cancel := context.WithCancel(ctx)
			defer cancel()

			go func() {
				if err := GetFrameLogForDevice(cctx, devEUI, logChannel); err != nil {
					log.Fatal(err)
				}
			}()

			// some time to subscribe
			time.Sleep(time.Millisecond * 100)

			Convey("When calling LogUplinkFrameForDevEUI", func() {
				rxPacket := models.RXPacket{
					RXInfoSet: []models.RXInfo{
						{
							MAC: mac,
						},
					},
				}
				So(LogUplinkFrameForDevEUI(devEUI, rxPacket), ShouldBeNil)

				Convey("Then the frame has been logged", func() {
					So(<-logChannel, ShouldResemble, FrameLog{
						UplinkFrame: &UplinkFrameLog{
							RXInfoSet: []models.RXInfo{
								{
									MAC: mac,
								},
							},
						},
					})
				})
			})

			Convey("When calling LogDownlinkFrameForDevEUI", func() {
				frameLog := DownlinkFrameLog{
					TXInfo: gw.TXInfo{
						MAC: mac,
					},
				}
				So(LogDownlinkFrameForDevEUI(devEUI, frameLog), ShouldBeNil)

				Convey("Then the frame has been logged", func() {
					So(<-logChannel, ShouldResemble, FrameLog{
						DownlinkFrame: &frameLog,
					})
				})
			})
		})
	})
}
