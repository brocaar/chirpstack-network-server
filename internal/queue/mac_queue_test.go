package queue

import (
	"testing"

	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/session"
	"github.com/brocaar/loraserver/internal/test"
	. "github.com/smartystreets/goconvey/convey"
)

func TestMACPayloadTXQueue(t *testing.T) {
	conf := test.GetConfig()

	Convey("Given a clean Redis database", t, func() {
		p := common.NewRedisPool(conf.RedisURL)
		test.MustFlushRedis(p)

		Convey("Given a node-session", func() {
			ns := session.NodeSession{
				DevAddr: [4]byte{1, 2, 3, 4},
				DevEUI:  [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
			}
			So(session.CreateNodeSession(p, ns), ShouldBeNil)

			Convey("When adding MACPayload a and b to the queue", func() {
				a := MACPayload{
					DevEUI: ns.DevEUI,
					Data:   []byte{1},
				}
				b := MACPayload{
					DevEUI: ns.DevEUI,
					Data:   []byte{2},
				}
				So(AddMACPayloadToTXQueue(p, a), ShouldBeNil)
				So(AddMACPayloadToTXQueue(p, b), ShouldBeNil)

				Convey("Then readMACPayloadTXQueue returns both MACPayload in the correct order", func() {
					payloads, err := ReadMACPayloadTXQueue(p, ns.DevAddr)
					So(err, ShouldBeNil)
					So(payloads, ShouldResemble, []MACPayload{a, b})
				})

				Convey("When deleting MACPayload a", func() {
					So(DeleteMACPayloadFromTXQueue(p, ns.DevAddr, a), ShouldBeNil)

					Convey("Then only MACPayload b is in the queue", func() {
						payloads, err := ReadMACPayloadTXQueue(p, ns.DevAddr)
						So(err, ShouldBeNil)
						So(payloads, ShouldResemble, []MACPayload{b})
					})
				})
			})
		})

	})
}

func TestFilterMACPayloads(t *testing.T) {
	Convey("Given a set of MACPayload items", t, func() {
		a := MACPayload{
			FRMPayload: false,
			Data:       []byte{1, 2, 3, 4, 5},
		}
		b := MACPayload{
			FRMPayload: false,
			Data:       []byte{9, 8, 7},
		}
		c := MACPayload{
			FRMPayload: true,
			Data:       []byte{9, 8, 7, 6, 5, 4, 3, 2, 1},
		}
		d := MACPayload{
			FRMPayload: true,
			Data:       []byte{1, 2, 3, 4, 5, 6, 7, 8, 9},
		}
		allPayloads := []MACPayload{a, b, c, d}

		Convey("When filtering on 15 bytes and FRMPayload=false", func() {
			payloads := FilterMACPayloads(allPayloads, false, 15)
			Convey("Then the expected set is returned", func() {
				So(payloads, ShouldResemble, []MACPayload{a, b})
			})
		})

		Convey("When filtering on 15 bytes and FRMPayload=true", func() {
			payloads := FilterMACPayloads(allPayloads, true, 15)
			Convey("Then the expected set is returned", func() {
				So(payloads, ShouldResemble, []MACPayload{c})
			})
		})

		Convey("Whe filtering on 100 bytes and FRMPayload=true", func() {
			payloads := FilterMACPayloads(allPayloads, true, 100)
			So(payloads, ShouldResemble, []MACPayload{c, d})
		})
	})
}
