package channels

import (
	"fmt"
	"testing"

	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/maccommand"
	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/session"
	"github.com/brocaar/loraserver/internal/test"
	"github.com/brocaar/lorawan"
	. "github.com/smartystreets/goconvey/convey"
)

func TestHandleChannelReconfigure(t *testing.T) {
	conf := test.GetConfig()

	Convey("Given a clean Redis database and a set of tests", t, func() {
		p := common.NewRedisPool(conf.RedisURL)
		test.MustFlushRedis(p)
		ctx := common.Context{
			RedisPool: p,
		}

		rxPacket := models.RXPacket{
			RXInfoSet: models.RXInfoSet{
				{DataRate: common.Band.DataRates[3]},
			},
		}

		tests := []struct {
			Name            string
			NodeSession     session.NodeSession
			Pending         *maccommand.Block
			ExpectedQueue   []maccommand.Block
			ExpectedPending *maccommand.Block
		}{
			{
				Name: "no channels to reconfigure",
				NodeSession: session.NodeSession{
					TXPower:         14,
					NbTrans:         2,
					EnabledChannels: []int{0, 1, 2},
				},
				ExpectedQueue:   nil,
				ExpectedPending: nil,
			},
			{
				Name: "channels to reconfigure",
				NodeSession: session.NodeSession{
					TXPower:         14,
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
					So(maccommand.SetPending(ctx.RedisPool, test.NodeSession.DevEUI, *test.Pending), ShouldBeNil)
				}

				So(HandleChannelReconfigure(ctx, test.NodeSession, rxPacket), ShouldBeNil)

				if test.ExpectedPending != nil {
					Convey("Then the expected mac-command block is set to pending", func() {
						pending, err := maccommand.ReadPending(ctx.RedisPool, test.NodeSession.DevEUI, lorawan.LinkADRReq)
						So(err, ShouldBeNil)
						So(pending, ShouldResemble, test.ExpectedPending)
					})
				}

				Convey("Then the expected mac-commands are in the queue", func() {
					queue, err := maccommand.ReadQueueItems(ctx.RedisPool, test.NodeSession.DevEUI)
					So(err, ShouldBeNil)
					So(test.ExpectedQueue, ShouldResemble, queue)
				})
			})
		}
	})
}
