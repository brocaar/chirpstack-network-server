package channels

import (
	"fmt"
	"testing"

	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/maccommand"
	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/loraserver/internal/test"
	"github.com/brocaar/lorawan"
	. "github.com/smartystreets/goconvey/convey"
)

func TestHandleChannelReconfigure(t *testing.T) {
	conf := test.GetConfig()

	Convey("Given a clean Redis database and a set of tests", t, func() {
		common.RedisPool = common.NewRedisPool(conf.RedisURL)
		test.MustFlushRedis(common.RedisPool)

		rxPacket := models.RXPacket{
			TXInfo: models.TXInfo{
				DataRate: common.Band.DataRates[3],
			},
		}

		tests := []struct {
			Name            string
			DeviceSession   storage.DeviceSession
			Pending         *maccommand.Block
			ExpectedQueue   []maccommand.Block
			ExpectedPending *maccommand.Block
		}{
			{
				Name: "no channels to reconfigure",
				DeviceSession: storage.DeviceSession{
					TXPowerIndex:    1,
					NbTrans:         2,
					EnabledChannels: []int{0, 1, 2},
				},
				ExpectedQueue:   nil,
				ExpectedPending: nil,
			},
			{
				Name: "channels to reconfigure",
				DeviceSession: storage.DeviceSession{
					TXPowerIndex:    1,
					NbTrans:         2,
					EnabledChannels: []int{0, 1}, // this is not realistic but good enough for testing
				},
				ExpectedQueue: []maccommand.Block{
					{
						CID: lorawan.LinkADRReq,
						MACCommands: maccommand.MACCommands{
							lorawan.MACCommand{
								CID: lorawan.LinkADRReq,
								Payload: &lorawan.LinkADRReqPayload{
									DataRate: 3,
									TXPower:  1,
									ChMask:   lorawan.ChMask{true, true, true},
									Redundancy: lorawan.Redundancy{
										NbRep: 2,
									},
								},
							},
						},
					},
				},
			},
		}

		for i, test := range tests {
			Convey(fmt.Sprintf("test: %s [%d]", test.Name, i), func() {
				if test.Pending != nil {
					So(maccommand.SetPending(common.RedisPool, test.DeviceSession.DevEUI, *test.Pending), ShouldBeNil)
				}

				So(HandleChannelReconfigure(test.DeviceSession, rxPacket), ShouldBeNil)

				if test.ExpectedPending != nil {
					Convey("Then the expected mac-command block is set to pending", func() {
						pending, err := maccommand.ReadPending(common.RedisPool, test.DeviceSession.DevEUI, lorawan.LinkADRReq)
						So(err, ShouldBeNil)
						So(pending, ShouldResemble, test.ExpectedPending)
					})
				}

				Convey("Then the expected mac-commands are in the queue", func() {
					queue, err := maccommand.ReadQueueItems(common.RedisPool, test.DeviceSession.DevEUI)
					So(err, ShouldBeNil)
					So(test.ExpectedQueue, ShouldResemble, queue)
				})
			})
		}
	})
}
