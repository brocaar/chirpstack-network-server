package framelog

import (
	"context"
	"log"
	"testing"
	"time"

	"github.com/brocaar/lorawan"

	commonPB "github.com/brocaar/loraserver/api/common"
	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/test"
	. "github.com/smartystreets/goconvey/convey"
)

func TestFrameLog(t *testing.T) {
	conf := test.GetConfig()
	p := common.NewRedisPool(conf.RedisURL, 10, 0)
	config.C.Redis.Pool = p

	mac := lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}
	devEUI := lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1}

	Convey("Given a clean Redis database", t, func() {
		test.MustFlushRedis(config.C.Redis.Pool)

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
				uplinkFrameSet := gw.UplinkFrameSet{
					PhyPayload: []byte{1, 2, 3, 4},
					TxInfo: &gw.UplinkTXInfo{
						Frequency:  868100000,
						Modulation: commonPB.Modulation_LORA,
						ModulationInfo: &gw.UplinkTXInfo_LoraModulationInfo{
							LoraModulationInfo: &gw.LoRaModulationInfo{
								SpreadingFactor: 7,
							},
						},
					},
					RxInfo: []*gw.UplinkRXInfo{
						{
							GatewayId: mac[:],
							LoraSnr:   5.5,
						},
					},
				}
				So(LogUplinkFrameForGateways(uplinkFrameSet), ShouldBeNil)

				Convey("Then the frame has been logged", func() {
					So(<-logChannel, ShouldResemble, FrameLog{
						UplinkFrame: &gw.UplinkFrameSet{
							PhyPayload: []byte{1, 2, 3, 4},
							TxInfo: &gw.UplinkTXInfo{
								Frequency:  868100000,
								Modulation: commonPB.Modulation_LORA,
								ModulationInfo: &gw.UplinkTXInfo_LoraModulationInfo{
									LoraModulationInfo: &gw.LoRaModulationInfo{
										SpreadingFactor: 7,
									},
								},
							},
							RxInfo: []*gw.UplinkRXInfo{
								{
									GatewayId: mac[:],
									LoraSnr:   5.5,
								},
							},
						},
					})
				})
			})

			Convey("When calling LogDownlinkFrameForGateway", func() {
				downlinkFrame := gw.DownlinkFrame{
					PhyPayload: []byte{1, 2, 3, 4},
					TxInfo: &gw.DownlinkTXInfo{
						GatewayId: mac[:],
					},
				}
				So(LogDownlinkFrameForGateway(downlinkFrame), ShouldBeNil)
				downlinkFrame.TxInfo.XXX_sizecache = 0

				Convey("Then the frame has been logged", func() {
					So(<-logChannel, ShouldResemble, FrameLog{
						DownlinkFrame: &downlinkFrame,
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
				uplinkFrameSet := gw.UplinkFrameSet{
					PhyPayload: []byte{1, 2, 3, 4},
					TxInfo: &gw.UplinkTXInfo{
						Frequency:  868100000,
						Modulation: commonPB.Modulation_LORA,
						ModulationInfo: &gw.UplinkTXInfo_LoraModulationInfo{
							LoraModulationInfo: &gw.LoRaModulationInfo{
								SpreadingFactor: 7,
							},
						},
					},
					RxInfo: []*gw.UplinkRXInfo{
						{
							GatewayId: mac[:],
							LoraSnr:   5.5,
						},
					},
				}
				So(LogUplinkFrameForDevEUI(devEUI, uplinkFrameSet), ShouldBeNil)

				Convey("Then the frame has been logged", func() {
					So(<-logChannel, ShouldResemble, FrameLog{
						UplinkFrame: &gw.UplinkFrameSet{
							PhyPayload: []byte{1, 2, 3, 4},
							TxInfo: &gw.UplinkTXInfo{
								Frequency:  868100000,
								Modulation: commonPB.Modulation_LORA,
								ModulationInfo: &gw.UplinkTXInfo_LoraModulationInfo{
									LoraModulationInfo: &gw.LoRaModulationInfo{
										SpreadingFactor: 7,
									},
								},
							},
							RxInfo: []*gw.UplinkRXInfo{
								{
									GatewayId: mac[:],
									LoraSnr:   5.5,
								},
							},
						},
					})
				})
			})

			Convey("When calling LogDownlinkFrameForDevEUI", func() {
				downlinkFrame := gw.DownlinkFrame{
					PhyPayload: []byte{1, 2, 3, 4},
					TxInfo: &gw.DownlinkTXInfo{
						GatewayId: mac[:],
					},
				}
				So(LogDownlinkFrameForDevEUI(devEUI, downlinkFrame), ShouldBeNil)
				downlinkFrame.TxInfo.XXX_sizecache = 0

				Convey("Then the frame has been logged", func() {
					So(<-logChannel, ShouldResemble, FrameLog{
						DownlinkFrame: &downlinkFrame,
					})
				})
			})
		})
	})
}
