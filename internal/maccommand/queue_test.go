package maccommand

import (
	"testing"

	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/session"
	"github.com/brocaar/loraserver/internal/test"
	"github.com/brocaar/lorawan"
	. "github.com/smartystreets/goconvey/convey"
)

func TestQueue(t *testing.T) {
	conf := test.GetConfig()

	Convey("Given a clean Redis database", t, func() {
		p := common.NewRedisPool(conf.RedisURL)
		test.MustFlushRedis(p)

		Convey("Given a node-session", func() {
			ns := session.NodeSession{
				DevAddr: [4]byte{1, 2, 3, 4},
				DevEUI:  [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
			}
			So(session.SaveNodeSession(p, ns), ShouldBeNil)

			Convey("When adding mac-command a and b to the queue", func() {
				a := Block{
					CID: lorawan.LinkADRReq,
					MACCommands: []lorawan.MACCommand{
						{
							CID:     lorawan.LinkADRReq,
							Payload: &lorawan.LinkADRReqPayload{DataRate: 1},
						},
					},
				}
				b := Block{
					CID: lorawan.RXParamSetupReq,
					MACCommands: []lorawan.MACCommand{
						{
							CID:     lorawan.RXParamSetupReq,
							Payload: &lorawan.RX2SetupReqPayload{Frequency: 868100000},
						},
					},
				}
				So(AddToQueue(p, ns.DevEUI, a), ShouldBeNil)
				So(AddToQueue(p, ns.DevEUI, b), ShouldBeNil)

				Convey("Then reading the queue returns both mac-command blocks in the correct order", func() {
					blocks, err := ReadQueue(p, ns.DevEUI)
					So(err, ShouldBeNil)
					So(blocks, ShouldResemble, []Block{a, b})
				})

				Convey("When deleting mac-command a", func() {
					So(DeleteQueueItem(p, ns.DevEUI, a), ShouldBeNil)

					Convey("Then only mac-command b is in the queue", func() {
						blocks, err := ReadQueue(p, ns.DevEUI)
						So(err, ShouldBeNil)
						So(blocks, ShouldResemble, []Block{b})
					})
				})
			})
		})

	})
}

func TestPending(t *testing.T) {
	conf := test.GetConfig()

	Convey("Given a clean Redis database", t, func() {
		p := common.NewRedisPool(conf.RedisURL)
		test.MustFlushRedis(p)

		Convey("When setting two mac-commands as pending", func() {
			devEUI := [8]byte{1, 2, 3, 4, 5, 6, 7, 8}
			a := Block{
				CID: lorawan.LinkADRReq,
				MACCommands: []lorawan.MACCommand{
					{
						CID:     lorawan.LinkADRReq,
						Payload: &lorawan.LinkADRReqPayload{DataRate: 1},
					},
				},
			}
			b := Block{
				CID: lorawan.LinkADRReq,
				MACCommands: []lorawan.MACCommand{
					{
						CID:     lorawan.LinkADRReq,
						Payload: &lorawan.LinkADRReqPayload{DataRate: 2},
					},
				},
			}

			So(SetPending(p, devEUI, a), ShouldBeNil)

			Convey("Then ReadPending returns the same block", func() {
				block, err := ReadPending(p, devEUI, lorawan.LinkADRReq)
				So(err, ShouldBeNil)
				So(block, ShouldResemble, &a)
			})

			Convey("Then ReadPending for a different CID returns nil", func() {
				block, err := ReadPending(p, devEUI, lorawan.DutyCycleReq)
				So(err, ShouldBeNil)
				So(block, ShouldBeNil)
			})

			Convey("Then ReadPending for a different DevEUI returns 0 items", func() {
				block, err := ReadPending(p, [8]byte{8, 7, 6, 5, 4, 3, 2, 1}, lorawan.LinkADRReq)
				So(err, ShouldBeNil)
				So(block, ShouldBeNil)
			})

			Convey("When overwriting the mac-commands for the same CID", func() {
				So(SetPending(p, devEUI, b), ShouldBeNil)

				Convey("Then only the new mac-commands are returned", func() {
					block, err := ReadPending(p, devEUI, lorawan.LinkADRReq)
					So(err, ShouldBeNil)
					So(block, ShouldResemble, &b)
				})
			})
		})
	})
}

func TestFilterItems(t *testing.T) {
	Convey("Given a set of mac-command items", t, func() {
		// 5 bytes
		a := Block{
			CID: lorawan.LinkADRReq,
			MACCommands: []lorawan.MACCommand{
				{
					CID:     lorawan.LinkADRReq,
					Payload: &lorawan.LinkADRReqPayload{DataRate: 1},
				},
			},
		}
		// 2 bytes
		b := Block{
			CID: lorawan.DutyCycleReq,
			MACCommands: []lorawan.MACCommand{
				{
					CID:     lorawan.DutyCycleReq,
					Payload: &lorawan.DutyCycleReqPayload{MaxDCCycle: 1},
				},
			},
		}
		// 5 bytes
		c := Block{
			CID: lorawan.RXParamSetupReq,
			MACCommands: []lorawan.MACCommand{
				{
					CID:     lorawan.RXParamSetupReq,
					Payload: &lorawan.RX2SetupReqPayload{Frequency: 868100000},
				},
			},
		}
		// 1 byte
		d := Block{
			FRMPayload: true,
			CID:        lorawan.DevStatusReq,
			MACCommands: []lorawan.MACCommand{
				{
					CID: lorawan.DevStatusReq,
				},
			},
		}
		// 6 bytes
		e := Block{
			CID: lorawan.NewChannelReq,
			MACCommands: []lorawan.MACCommand{
				{
					CID:     lorawan.NewChannelReq,
					Payload: &lorawan.NewChannelReqPayload{ChIndex: 3, Freq: 868900000, MinDR: 0, MaxDR: 5},
				},
			},
		}

		allBlocks := []Block{a, b, c, d, e}

		Convey("When filtering on 15 bytes and FRMPayload=false", func() {
			blocks, err := FilterItems(allBlocks, false, 15)
			So(err, ShouldBeNil)
			Convey("Then the expected set is returned", func() {
				So(blocks, ShouldResemble, []Block{a, b, c})
			})
		})

		Convey("When filtering on 15 bytes and FRMPayload=true", func() {
			blocks, err := FilterItems(allBlocks, true, 15)
			So(err, ShouldBeNil)
			Convey("Then the expected set is returned", func() {
				So(blocks, ShouldResemble, []Block{d})
			})
		})

		Convey("Whe filtering on 100 bytes and FRMPayload=true", func() {
			blocks, err := FilterItems(allBlocks, false, 100)
			So(err, ShouldBeNil)
			Convey("Then the expected set is returned", func() {
				So(blocks, ShouldResemble, []Block{a, b, c, e})
			})
		})
	})
}
