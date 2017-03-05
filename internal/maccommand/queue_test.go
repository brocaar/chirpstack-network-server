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
				a := QueueItem{
					DevEUI: ns.DevEUI,
					Data:   []byte{1},
				}
				b := QueueItem{
					DevEUI: ns.DevEUI,
					Data:   []byte{2},
				}
				So(AddToQueue(p, a), ShouldBeNil)
				So(AddToQueue(p, b), ShouldBeNil)

				Convey("Then reading the queue returns both mac-payloads in the correct order", func() {
					payloads, err := ReadQueue(p, ns.DevEUI)
					So(err, ShouldBeNil)
					So(payloads, ShouldResemble, []QueueItem{a, b})
				})

				Convey("When deleting mac-command a", func() {
					So(DeleteQueueItem(p, ns.DevEUI, a), ShouldBeNil)

					Convey("Then only mac-command b is in the queue", func() {
						payloads, err := ReadQueue(p, ns.DevEUI)
						So(err, ShouldBeNil)
						So(payloads, ShouldResemble, []QueueItem{b})
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
			commands := []lorawan.MACCommandPayload{
				&lorawan.LinkADRReqPayload{DataRate: 1, TXPower: 2},
				&lorawan.LinkADRReqPayload{DataRate: 3, TXPower: 4},
			}

			devEUI := [8]byte{1, 2, 3, 4, 5, 6, 7, 8}
			So(SetPending(p, devEUI, lorawan.LinkADRReq, commands), ShouldBeNil)

			Convey("Then ReadPending returns the same mac-commands", func() {
				out, err := ReadPending(p, devEUI, lorawan.LinkADRReq)
				So(err, ShouldBeNil)
				So(out, ShouldResemble, commands)
			})

			Convey("Then ReadPending for a different CID returns 0 items", func() {
				out, err := ReadPending(p, devEUI, lorawan.DutyCycleReq)
				So(err, ShouldBeNil)
				So(out, ShouldHaveLength, 0)
			})

			Convey("Then ReadPending for a different DevEUI returns 0 items", func() {
				out, err := ReadPending(p, [8]byte{8, 7, 6, 5, 4, 3, 2, 1}, lorawan.LinkADRReq)
				So(err, ShouldBeNil)
				So(out, ShouldHaveLength, 0)
			})

			Convey("When overwriting the mac-commands for the same CID", func() {
				commands := []lorawan.MACCommandPayload{
					&lorawan.LinkADRReqPayload{DataRate: 5, TXPower: 6},
				}
				So(SetPending(p, devEUI, lorawan.LinkADRReq, commands), ShouldBeNil)

				Convey("Then only the new mac-commands are returned", func() {
					out, err := ReadPending(p, devEUI, lorawan.LinkADRReq)
					So(err, ShouldBeNil)
					So(out, ShouldResemble, commands)
				})
			})
		})
	})
}

func TestFilterItems(t *testing.T) {
	Convey("Given a set of mac-command items", t, func() {
		a := QueueItem{
			FRMPayload: false,
			Data:       []byte{1, 2, 3, 4, 5},
		}
		b := QueueItem{
			FRMPayload: false,
			Data:       []byte{9, 8, 7},
		}
		c := QueueItem{
			FRMPayload: true,
			Data:       []byte{9, 8, 7, 6, 5, 4, 3, 2, 1},
		}
		d := QueueItem{
			FRMPayload: true,
			Data:       []byte{1, 2, 3, 4, 5, 6, 7, 8, 9},
		}
		allPayloads := []QueueItem{a, b, c, d}

		Convey("When filtering on 15 bytes and FRMPayload=false", func() {
			payloads := FilterItems(allPayloads, false, 15)
			Convey("Then the expected set is returned", func() {
				So(payloads, ShouldResemble, []QueueItem{a, b})
			})
		})

		Convey("When filtering on 15 bytes and FRMPayload=true", func() {
			payloads := FilterItems(allPayloads, true, 15)
			Convey("Then the expected set is returned", func() {
				So(payloads, ShouldResemble, []QueueItem{c})
			})
		})

		Convey("Whe filtering on 100 bytes and FRMPayload=true", func() {
			payloads := FilterItems(allPayloads, true, 100)
			So(payloads, ShouldResemble, []QueueItem{c, d})
		})
	})
}
