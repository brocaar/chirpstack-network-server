package maccommand

import (
	"errors"
	"testing"

	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/session"
	"github.com/brocaar/loraserver/internal/test"
	"github.com/brocaar/lorawan"
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
				DevEUI: [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
			}
			So(session.SaveNodeSession(p, ns), ShouldBeNil)

			Convey("Testing LinkADRAns", func() {
				linkADRReq := &lorawan.LinkADRReqPayload{
					TXPower: 3,
					Redundancy: lorawan.Redundancy{
						NbRep: 2,
					},
				}
				linkADRAnsPL := &lorawan.LinkADRAnsPayload{
					ChannelMaskACK: true,
					DataRateACK:    true,
					PowerACK:       true,
				}
				linkADRAns := lorawan.MACCommand{
					CID:     lorawan.LinkADRAns,
					Payload: linkADRAnsPL,
				}

				Convey("Given a pending linkADRReq and positive ack", func() {
					So(SetPending(p, ns.DevEUI, lorawan.LinkADRReq, []lorawan.MACCommandPayload{linkADRReq}), ShouldBeNil)
					So(Handle(ctx, &ns, linkADRAns), ShouldBeNil)

					Convey("Then the node-session TXPower and NbTrans are updated correctly", func() {
						So(ns.TXPower, ShouldEqual, common.Band.TXPower[3])
						So(ns.NbTrans, ShouldEqual, 2)
					})
				})

				Convey("Given a pending linkADRReq and negative ack", func() {
					linkADRAnsPL.ChannelMaskACK = false
					So(SetPending(p, ns.DevEUI, lorawan.LinkADRReq, []lorawan.MACCommandPayload{linkADRReq}), ShouldBeNil)
					So(Handle(ctx, &ns, linkADRAns), ShouldBeNil)

					Convey("Then the node-session TXPower and NbTrans are not updated", func() {
						So(ns.TXPower, ShouldEqual, 0)
						So(ns.NbTrans, ShouldEqual, 0)
					})
				})

				Convey("Given no pending linkADRReq and positive ack", func() {
					err := Handle(ctx, &ns, linkADRAns)
					Convey("Then an error is returned", func() {
						So(err, ShouldResemble, errors.New("no pending adr requests found"))
					})
				})
			})
		})
	})
}
