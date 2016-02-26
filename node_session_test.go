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
