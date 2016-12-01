package session

import (
	"errors"
	"fmt"
	"testing"

	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/test"
	"github.com/brocaar/lorawan"
	. "github.com/smartystreets/goconvey/convey"
)

func TestGetRandomDevAddr(t *testing.T) {
	conf := test.GetConfig()

	Convey("Given a Redis database and NetID 010203", t, func() {
		p := common.NewRedisPool(conf.RedisURL)
		test.MustFlushRedis(p)
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

func TestNodeSession(t *testing.T) {
	conf := test.GetConfig()

	Convey("Given a clean Redis database", t, func() {
		p := common.NewRedisPool(conf.RedisURL)
		test.MustFlushRedis(p)

		Convey("Given a NodeSession", func() {
			ns := NodeSession{
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
					{0, 1, 1, true},                                                               // one packet was lost
					{1, 1, 1, true},                                                               // ideal case, the FCnt has the expected value
					{2, 1, 0, false},                                                              // old packet received or re-transmission
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
