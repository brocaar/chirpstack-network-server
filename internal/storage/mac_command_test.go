package storage

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/brocaar/chirpstack-network-server/internal/test"
	"github.com/brocaar/lorawan"
)

func TestMACCommand(t *testing.T) {
	conf := test.GetConfig()
	ctx := context.Background()

	devEUI := lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}
	macCommands := []MACCommandBlock{
		{
			CID: lorawan.LinkADRReq,
			MACCommands: []lorawan.MACCommand{
				{
					CID:     lorawan.LinkADRReq,
					Payload: &lorawan.LinkADRReqPayload{DataRate: 1},
				},
			},
		},
		{
			CID: lorawan.RXParamSetupReq,
			MACCommands: []lorawan.MACCommand{
				{
					CID:     lorawan.RXParamSetupReq,
					Payload: &lorawan.RXParamSetupReqPayload{Frequency: 868100000},
				},
			},
		},
	}

	Convey("Given a clean Redis database", t, func() {
		if err := Setup(conf); err != nil {
			t.Fatal(err)
		}
		RedisClient().FlushAll()

		Convey("When adding two items to the queue", func() {
			for _, m := range macCommands {
				So(CreateMACCommandQueueItem(context.Background(), devEUI, m), ShouldBeNil)
			}

			Convey("Then reading the queue returns both mac-commands in the correct order", func() {
				blocks, err := GetMACCommandQueueItems(context.Background(), devEUI)
				So(err, ShouldBeNil)
				So(blocks, ShouldResemble, macCommands)
			})

			Convey("When deleting a mac-command", func() {
				So(DeleteMACCommandQueueItem(context.Background(), devEUI, macCommands[0]), ShouldBeNil)

				Convey("Then the item has been removed from the queue", func() {
					blocks, err := GetMACCommandQueueItems(context.Background(), devEUI)
					So(err, ShouldBeNil)
					So(blocks, ShouldResemble, macCommands[1:])
				})
			})

			Convey("When flushing the mac-command queue", func() {
				So(FlushMACCommandQueue(ctx, devEUI), ShouldBeNil)

				Convey("Then the queue is empty", func() {
					blocks, err := GetMACCommandQueueItems(context.Background(), devEUI)
					So(err, ShouldBeNil)
					So(blocks, ShouldHaveLength, 0)
				})
			})
		})

		Convey("When setting a pending mac-command", func() {
			So(SetPendingMACCommand(context.Background(), devEUI, macCommands[0]), ShouldBeNil)

			Convey("Then the pending mac-command can be retrieved", func() {
				block, err := GetPendingMACCommand(context.Background(), devEUI, macCommands[0].CID)
				So(err, ShouldBeNil)
				So(*block, ShouldResemble, macCommands[0])
			})

			Convey("When deleting a pending mac-command", func() {
				So(DeletePendingMACCommand(context.Background(), devEUI, macCommands[0].CID), ShouldBeNil)

				Convey("Then it has been removed", func() {
					block, err := GetPendingMACCommand(context.Background(), devEUI, macCommands[0].CID)
					So(err, ShouldBeNil)
					So(block, ShouldBeNil)
				})
			})
		})
	})
}
