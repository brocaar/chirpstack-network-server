package storage

import (
	"errors"
	"fmt"
	"testing"

	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/models"
	"github.com/brocaar/lorawan"
	. "github.com/smartystreets/goconvey/convey"
)

func TestNodeSession(t *testing.T) {
	conf := common.GetTestConfig()

	Convey("Given a clean Redis database", t, func() {
		p := NewRedisPool(conf.RedisURL)
		common.MustFlushRedis(p)

		Convey("Given a NodeSession", func() {
			ns := models.NodeSession{
				DevAddr: [4]byte{1, 2, 3, 4},
				DevEUI:  [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
			}

			Convey("When getting a non-existing NodeSession", func() {
				_, err := GetNodeSession(p, ns.DevAddr)
				Convey("Then an error is returned", func() {
					So(err, ShouldResemble, errors.New("get node-session for DevAddr 01020304 error: redigo: nil returned"))
				})
			})

			Convey("When saving the NodeSession", func() {
				So(SaveNodeSession(p, ns), ShouldBeNil)

				Convey("Then when getting the NodeSession, the same data is returned", func() {
					ns2, err := GetNodeSession(p, ns.DevAddr)
					So(err, ShouldBeNil)
					So(ns2, ShouldResemble, ns)
				})

				Convey("Then the session can be retrieved by it's DevEUI", func() {
					ns2, err := GetNodeSessionByDevEUI(p, ns.DevEUI)
					So(err, ShouldBeNil)
					So(ns2, ShouldResemble, ns)
				})
			})

			Convey("When calling validateAndGetFullFCntUp", func() {
				testTable := []struct {
					ServerFCnt uint32
					NodeFCnt   uint32
					FullFCnt   uint32
					Valid      bool
				}{
					{0, 1, 1, true},                                                               // ideal case counter was incremented
					{1, 1, 1, true},                                                               // re-transmission
					{2, 1, 0, false},                                                              // old packet received
					{0, common.Band.MaxFCntGap, 0, false},                                         // gap should be less than MaxFCntGap
					{0, common.Band.MaxFCntGap - 1, common.Band.MaxFCntGap - 1, true},             // gap is exactly within the allowed MaxFCntGap
					{65536, common.Band.MaxFCntGap - 1, common.Band.MaxFCntGap - 1 + 65536, true}, // roll-over happened, gap ix exactly within allowed MaxFCntGap
					{65535, common.Band.MaxFCntGap, 0, false},                                     // roll-over happened, but too many lost frames
					{65535, 0, 65536, true},                                                       // roll-over happened
					{65536, 0, 65536, true},                                                       // re-transmission
					{4294967295, 0, 0, true},                                                      // 32 bit roll-over happened, counter started at 0 again
				}

				for _, test := range testTable {
					Convey(fmt.Sprintf("Then when FCntUp=%d, ValidateAndGetFullFCntUp(%d) should return (%d, %t)", test.ServerFCnt, test.NodeFCnt, test.FullFCnt, test.Valid), func() {
						ns.FCntUp = test.ServerFCnt
						fullFCntUp, ok := ValidateAndGetFullFCntUp(ns, test.NodeFCnt)
						So(ok, ShouldEqual, test.Valid)
						So(fullFCntUp, ShouldEqual, test.FullFCnt)
					})
				}
			})

		})
	})
}

func TestGetRandomDevAddr(t *testing.T) {
	conf := common.GetTestConfig()

	Convey("Given a Redis database and NetID 010203", t, func() {
		p := NewRedisPool(conf.RedisURL)
		common.MustFlushRedis(p)
		netID := lorawan.NetID{1, 2, 3}

		Convey("When calling getRandomDevAddr many times, it should always return an unique DevAddr", func() {
			log := make(map[lorawan.DevAddr]struct{})
			for i := 0; i < 1000; i++ {
				devAddr, err := GetRandomDevAddr(p, netID)
				if err != nil {
					t.Fatal(err)
				}
				if devAddr.NwkID() != netID.NwkID() {
					t.Fatalf("%b must equal %b", devAddr.NwkID(), netID.NwkID())
				}
				if len(log) != i {
					t.Fatalf("%d must equal %d", len(log), i)
				}
				log[devAddr] = struct{}{}
			}
		})
	})
}

func TestMACPayloadTXQueue(t *testing.T) {
	conf := common.GetTestConfig()

	Convey("Given a clean Redis database", t, func() {
		p := NewRedisPool(conf.RedisURL)
		common.MustFlushRedis(p)

		Convey("Given a node-session", func() {
			ns := models.NodeSession{
				DevAddr: [4]byte{1, 2, 3, 4},
				DevEUI:  [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
			}
			So(CreateNodeSession(p, ns), ShouldBeNil)

			Convey("When adding MACPayload a and b to the queue", func() {
				a := models.MACPayload{
					Reference: "a",
					DevEUI:    ns.DevEUI,
				}
				b := models.MACPayload{
					Reference: "b",
					DevEUI:    ns.DevEUI,
				}
				So(AddMACPayloadToTXQueue(p, a), ShouldBeNil)
				So(AddMACPayloadToTXQueue(p, b), ShouldBeNil)

				Convey("Then readMACPayloadTXQueue returns both MACPayload in the correct order", func() {
					payloads, err := ReadMACPayloadTXQueue(p, ns.DevAddr)
					So(err, ShouldBeNil)
					So(payloads, ShouldResemble, []models.MACPayload{a, b})
				})

				Convey("When deleting MACPayload a", func() {
					So(DeleteMACPayloadFromTXQueue(p, ns.DevAddr, a), ShouldBeNil)

					Convey("Then only MACPayload b is in the queue", func() {
						payloads, err := ReadMACPayloadTXQueue(p, ns.DevAddr)
						So(err, ShouldBeNil)
						So(payloads, ShouldResemble, []models.MACPayload{b})
					})
				})
			})
		})

	})
}

func TestFilterMACPayloads(t *testing.T) {
	Convey("Given a set of MACPayload items", t, func() {
		a := models.MACPayload{
			FRMPayload: false,
			MACCommand: []byte{1, 2, 3, 4, 5},
		}
		b := models.MACPayload{
			FRMPayload: false,
			MACCommand: []byte{9, 8, 7},
		}
		c := models.MACPayload{
			FRMPayload: true,
			MACCommand: []byte{9, 8, 7, 6, 5, 4, 3, 2, 1},
		}
		d := models.MACPayload{
			FRMPayload: true,
			MACCommand: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9},
		}
		allPayloads := []models.MACPayload{a, b, c, d}

		Convey("When filtering on 15 bytes and FRMPayload=false", func() {
			payloads := FilterMACPayloads(allPayloads, false, 15)
			Convey("Then the expected set is returned", func() {
				So(payloads, ShouldResemble, []models.MACPayload{a, b})
			})
		})

		Convey("When filtering on 15 bytes and FRMPayload=true", func() {
			payloads := FilterMACPayloads(allPayloads, true, 15)
			Convey("Then the expected set is returned", func() {
				So(payloads, ShouldResemble, []models.MACPayload{c})
			})
		})

		Convey("Whe filtering on 100 bytes and FRMPayload=true", func() {
			payloads := FilterMACPayloads(allPayloads, true, 100)
			So(payloads, ShouldResemble, []models.MACPayload{c, d})
		})
	})
}
