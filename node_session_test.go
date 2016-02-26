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
		})
	})
}
