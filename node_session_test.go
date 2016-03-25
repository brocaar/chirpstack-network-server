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
				DevEUI:  [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
			}

			Convey("When getting a non-existing NodeSession", func() {
				_, err := getNodeSession(p, ns.DevAddr)
				Convey("Then an error is returned", func() {
					So(err, ShouldEqual, redis.ErrNil)
				})
			})

			Convey("When saving the NodeSession", func() {
				So(saveNodeSession(p, ns), ShouldBeNil)

				Convey("Then when getting the NodeSession, the same data is returned", func() {
					ns2, err := getNodeSession(p, ns.DevAddr)
					So(err, ShouldBeNil)
					So(ns2, ShouldResemble, ns)
				})

				Convey("Then the session can be retrieved by it's DevEUI", func() {
					ns2, err := getNodeSessionByDevEUI(p, ns.DevEUI)
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
					{0, 1, 1, true},                                                       // ideal case counter was incremented
					{1, 1, 1, true},                                                       // re-transmission
					{2, 1, 0, false},                                                      // old packet received
					{0, lorawan.MaxFCntGap, 0, false},                                     // gap should be less than MaxFCntGap
					{0, lorawan.MaxFCntGap - 1, lorawan.MaxFCntGap - 1, true},             // gap is exactly within the allowed MaxFCntGap
					{65536, lorawan.MaxFCntGap - 1, lorawan.MaxFCntGap - 1 + 65536, true}, // roll-over happened, gap ix exactly within allowed MaxFCntGap
					{65535, lorawan.MaxFCntGap, 0, false},                                 // roll-over happened, but too many lost frames
					{65535, 0, 65536, true},                                               // roll-over happened
					{65536, 0, 65536, true},                                               // re-transmission
					{4294967295, 0, 0, true},                                              // 32 bit roll-over happened, counter started at 0 again
				}

				for _, test := range testTable {
					Convey(fmt.Sprintf("Then when FCntUp=%d, ValidateAndGetFullFCntUp(%d) should return (%d, %t)", test.ServerFCnt, test.NodeFCnt, test.FullFCnt, test.Valid), func() {
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

func TestgetRandomDevAddr(t *testing.T) {
	conf := getConfig()

	Convey("Given a Redis database and NetID 010203", t, func() {
		p := NewRedisPool(conf.RedisURL)
		netID := lorawan.NetID{1, 2, 3}

		Convey("When calling getRandomDevAddr many times, it should always return an unique DevAddr", func() {
			log := make(map[lorawan.DevAddr]struct{})
			for i := 0; i < 1000; i++ {
				devAddr, err := getRandomDevAddr(p, netID)
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

func TestNodeSessionAPI(t *testing.T) {
	conf := getConfig()

	Convey("Given a clean database and an API instance", t, func() {
		db, err := OpenDatabase(conf.PostgresDSN)
		So(err, ShouldBeNil)
		mustResetDB(db)
		p := NewRedisPool(conf.RedisURL)
		mustFlushRedis(p)

		ctx := Context{
			DB:        db,
			RedisPool: p,
			NetID:     [3]byte{1, 2, 3},
		}

		api := NewNodeSessionAPI(ctx)

		Convey("Given an application and node are created (fk constraints)", func() {
			app := Application{
				AppEUI: [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
				Name:   "test app",
			}
			So(createApplication(ctx.DB, app), ShouldBeNil)

			node := Node{
				DevEUI:        [8]byte{8, 7, 6, 5, 4, 3, 2, 1},
				AppEUI:        [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
				AppKey:        [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
				UsedDevNonces: [][2]byte{},
			}
			So(createNode(ctx.DB, node), ShouldBeNil)

			ns := NodeSession{
				DevAddr:  [4]byte{6, 2, 3, 4},
				DevEUI:   node.DevEUI,
				AppEUI:   node.AppEUI,
				AppSKey:  node.AppKey,
				NwkSKey:  node.AppKey,
				FCntUp:   10,
				FCntDown: 11,
			}

			Convey("When calling Create", func() {
				var devAddr lorawan.DevAddr
				So(api.Create(ns, &devAddr), ShouldBeNil)
				So(devAddr, ShouldResemble, ns.DevAddr)

				Convey("Then the session has been created", func() {
					var ns2 NodeSession
					So(api.Get(ns.DevAddr, &ns2), ShouldBeNil)
					So(ns2, ShouldResemble, ns)

					Convey("Then the session can be deleted", func() {
						So(api.Delete(ns.DevAddr, &devAddr), ShouldBeNil)
						So(api.Get(ns.DevAddr, &ns2), ShouldNotBeNil)
					})
				})

				Convey("Then the session can be retrieved by DevEUI", func() {
					var ns2 NodeSession
					So(api.GetByDevEUI(ns.DevEUI, &ns2), ShouldBeNil)
					So(ns2, ShouldResemble, ns)
				})
			})

			Convey("When calling Create with a DevAddr which has a wrong NwkID", func() {
				var devAddr lorawan.DevAddr
				ns.DevAddr = [4]byte{1, 2, 3, 4}
				err := api.Create(ns, &devAddr)
				Convey("Then an error is returned", func() {
					So(err, ShouldNotBeNil)
				})
			})
		})
	})
}
