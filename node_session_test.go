package loraserver

import (
	"fmt"
	"testing"

	"github.com/brocaar/lorawan"
	"github.com/garyburd/redigo/redis"
	. "github.com/smartystreets/goconvey/convey"
)

func TestNodeSession(t *testing.T) {
	conf := getConfig()

	Convey("Given a clean Redis database", t, func() {
		p := NewRedisPool(conf.RedisURL)
		mustFlushRedis(p)

		Convey("Given a NodeSession", func() {
			ns := NodeSession{
				DevAddr: [4]byte{1, 2, 3, 4},
			}

			Convey("When getting a non-existing NodeSession", func() {
				_, err := GetNodeSession(p, ns.DevAddr)
				Convey("Then an error is returned", func() {
					So(err, ShouldEqual, redis.ErrNil)
				})
			})

			Convey("When saving the NodeSession", func() {
				So(SaveNodeSession(p, ns), ShouldBeNil)

				Convey("Then when getting the NodeSession, the same data is returned", func() {
					ns2, err := GetNodeSession(p, ns.DevAddr)
					So(err, ShouldBeNil)
					So(ns2, ShouldResemble, ns)
				})
			})

			Convey("When calling ValidateAndGetFullFCntUp", func() {
				testTable := []struct {
					ServerFCnt uint32
					NodeFCnt   uint32
					FullFCnt   uint32
					Valid      bool
				}{
					{1, 1, 1, true},                                                       // ideal case, no dropped frames
					{2, 1, 0, false},                                                      // old packet received
					{0, lorawan.MaxFCntGap, 0, false},                                     // gap should be less than MaxFCntGap
					{0, lorawan.MaxFCntGap - 1, lorawan.MaxFCntGap - 1, true},             // gap is exactly within the allowed MaxFCntGap
					{65536, lorawan.MaxFCntGap - 1, lorawan.MaxFCntGap - 1 + 65536, true}, // roll-over happened, gap ix exactly within allowed MaxFCntGap
					{65535, lorawan.MaxFCntGap, 0, false},                                 // roll-over happened, but too many lost frames
					{65535, 0, 65536, true},                                               // roll-over happened
					{65536, 0, 65536, true},                                               // ideal case, no dropped frames
					{4294967295, 0, 0, true},                                              // 32 bit roll-over happened, counter started at 0 again
				}

				for _, test := range testTable {
					Convey(fmt.Sprintf("Then when FCntUP=%d, ValidateAndGetFullFCntUp(%d) should return (%d, %t)", test.ServerFCnt, test.NodeFCnt, test.FullFCnt, test.Valid), func() {
						ns.FCntUp = test.ServerFCnt
						fullFCntUp, ok := ns.ValidateAndGetFullFCntUp(test.NodeFCnt)
						So(ok, ShouldEqual, test.Valid)
						So(fullFCntUp, ShouldEqual, test.FullFCnt)
					})
				}
			})

		})
	})
}

func TestNewNodeSessionsFromABP(t *testing.T) {
	conf := getConfig()

	Convey("Given a Redis and Postgres database", t, func() {
		p := NewRedisPool(conf.RedisURL)
		mustFlushRedis(p)
		db, err := OpenDatabase(conf.PostgresDSN)
		So(err, ShouldBeNil)
		mustResetDB(db)

		Convey("Given an Application, Node and NodeABP in the database", func() {
			app := Application{
				AppEUI: lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 1},
				Name:   "test app",
			}
			So(CreateApplication(db, app), ShouldBeNil)

			node := Node{
				DevEUI: lorawan.EUI64{2, 2, 2, 2, 2, 2, 2, 2},
				AppEUI: app.AppEUI,
				AppKey: lorawan.AES128Key{3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3},
			}
			So(CreateNode(db, node), ShouldBeNil)

			abp := NodeABP{
				DevAddr: lorawan.DevAddr{1, 2, 3, 4},
				DevEUI:  node.DevEUI,
				AppSKey: lorawan.AES128Key{4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4},
				NwkSKey: lorawan.AES128Key{5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5},
			}
			So(CreateNodeABP(db, abp), ShouldBeNil)

			Convey("When calling NewNodeSessionsFromABP", func() {
				So(NewNodeSessionsFromABP(db, p), ShouldBeNil)

				Convey("Then the NodeSession was created", func() {
					ns, err := GetNodeSession(p, abp.DevAddr)
					So(err, ShouldBeNil)
					So(ns, ShouldResemble, NodeSession{
						DevAddr:  abp.DevAddr,
						DevEUI:   abp.DevEUI,
						AppSKey:  abp.AppSKey,
						NwkSKey:  abp.NwkSKey,
						FCntUp:   0,
						FCntDown: 0,
						AppEUI:   app.AppEUI,
						AppKey:   node.AppKey,
					})
				})
			})
		})
	})
}
