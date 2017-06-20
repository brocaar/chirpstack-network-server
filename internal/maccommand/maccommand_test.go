package maccommand

import (
	"testing"

	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/session"
	"github.com/brocaar/loraserver/internal/test"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/band"
	"github.com/pkg/errors"
	. "github.com/smartystreets/goconvey/convey"
)

func TestHandle(t *testing.T) {
	conf := test.GetConfig()

	Convey("Given a clean Redis database", t, func() {
		p := common.NewRedisPool(conf.RedisURL)
		test.MustFlushRedis(p)

		ctx := common.Context{
			RedisPool: p,
		}

		Convey("Given a node-session", func() {
			ns := session.NodeSession{
				DevEUI:          [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
				EnabledChannels: []int{0, 1},
			}
			So(session.SaveNodeSession(p, ns), ShouldBeNil)

			Convey("Test LinkCheckReq", func() {
				block := Block{
					CID: lorawan.LinkCheckReq,
					MACCommands: MACCommands{
						lorawan.MACCommand{
							CID: lorawan.LinkCheckReq,
						},
					},
				}

				rxInfoSet := models.RXInfoSet{
					{
						LoRaSNR: 5,
						DataRate: band.DataRate{
							SpreadFactor: 10,
						},
					},
				}

				So(Handle(ctx, &ns, block, rxInfoSet), ShouldBeNil)

				Convey("Then the expected response was added to the mac-command queue", func() {
					items, err := ReadQueueItems(ctx.RedisPool, ns.DevEUI)
					So(err, ShouldBeNil)
					So(items, ShouldHaveLength, 1)
					So(items[0], ShouldResemble, Block{
						CID: lorawan.LinkCheckAns,
						MACCommands: MACCommands{
							{
								CID: lorawan.LinkCheckAns,
								Payload: &lorawan.LinkCheckAnsPayload{
									GwCnt:  1,
									Margin: 20, // 5 - -15 (see SpreadFactorToRequiredSNRTable)
								},
							},
						},
					})
				})
			})

			Convey("Testing LinkADRAns", func() {
				linkADRReqMAC := lorawan.MACCommand{
					CID: lorawan.LinkADRReq,
					Payload: &lorawan.LinkADRReqPayload{
						ChMask:  lorawan.ChMask{true, true, true},
						TXPower: 3,
						Redundancy: lorawan.Redundancy{
							NbRep: 2,
						},
					},
				}
				linkADRAnsPL := &lorawan.LinkADRAnsPayload{
					ChannelMaskACK: true,
					DataRateACK:    true,
					PowerACK:       true,
				}

				linkADRAns := Block{
					CID: lorawan.LinkADRAns,
					MACCommands: MACCommands{
						lorawan.MACCommand{
							CID:     lorawan.LinkADRAns,
							Payload: linkADRAnsPL,
						},
					},
				}

				Convey("Given a pending linkADRReq and positive ack", func() {
					So(SetPending(p, ns.DevEUI, Block{
						CID: linkADRAns.CID,
						MACCommands: []lorawan.MACCommand{
							linkADRReqMAC,
						},
					}), ShouldBeNil)
					So(Handle(ctx, &ns, linkADRAns, nil), ShouldBeNil)

					Convey("Then the node-session TXPower and NbTrans are updated correctly", func() {
						So(ns.TXPower, ShouldEqual, common.Band.TXPower[3])
						So(ns.NbTrans, ShouldEqual, 2)
					})

					Convey("Then the enabled channels on the node are updated", func() {
						So(ns.EnabledChannels, ShouldResemble, []int{0, 1, 2})
					})
				})

				Convey("Given a pending linkADRReq and negative ack", func() {
					linkADRAnsPL.ChannelMaskACK = false
					So(SetPending(p, ns.DevEUI, Block{
						CID: lorawan.LinkADRReq,
						MACCommands: []lorawan.MACCommand{
							linkADRReqMAC,
						},
					}), ShouldBeNil)
					So(Handle(ctx, &ns, linkADRAns, nil), ShouldBeNil)

					Convey("Then the node-session TXPower and NbTrans are not updated", func() {
						So(ns.TXPower, ShouldEqual, 0)
						So(ns.NbTrans, ShouldEqual, 0)
					})

					Convey("Then the enabled channels on the node are not updated", func() {
						So(ns.EnabledChannels, ShouldResemble, []int{0, 1})
					})
				})

				Convey("Given no pending linkADRReq and positive ack", func() {
					err := Handle(ctx, &ns, linkADRAns, nil)
					Convey("Then an error is returned", func() {
						So(errors.Cause(err), ShouldResemble, ErrDoesNotExist)
					})
				})
			})
		})
	})
}
