package storage

import (
	"testing"
	"time"

	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/models"
	. "github.com/smartystreets/goconvey/convey"
)

func TestTXPayloadQueue(t *testing.T) {
	conf := common.GetTestConfig()

	Convey("Given a clean Redis database", t, func() {
		p := NewRedisPool(conf.RedisURL)
		common.MustFlushRedis(p)

		Convey("Given two TXPayload structs (a and b) for the same DevEUI", func() {
			devEUI := [8]byte{1, 2, 3, 4, 5, 6, 7, 8}
			a := models.TXPayload{
				DevEUI: devEUI,
				Data:   []byte("hello!"),
			}
			b := models.TXPayload{
				DevEUI: devEUI,
				Data:   []byte("world"),
			}

			Convey("When getting an item from the non-existing queue", func() {
				_, err := GetTXPayloadFromQueue(p, devEUI)
				Convey("Then errEmptyQueue error is returned", func() {
					So(err, ShouldResemble, common.ErrEmptyQueue)
				})
			})

			Convey("Given the NodePayloadQueueTTL is 100 ms", func() {
				common.NodeTXPayloadQueueTTL = 100 * time.Millisecond

				Convey("Given struct a and b are pushed to the queue", func() {
					So(AddTXPayloadToQueue(p, a), ShouldBeNil)
					So(AddTXPayloadToQueue(p, b), ShouldBeNil)

					Convey("Then the queue size is 2", func() {
						count, err := GetTXPayloadQueueSize(p, devEUI)
						So(err, ShouldBeNil)
						So(count, ShouldEqual, 2)
					})

					Convey("Then after 150 ms the queue has expired", func() {
						time.Sleep(150 * time.Millisecond)
						_, err := GetTXPayloadFromQueue(p, devEUI)
						So(err, ShouldResemble, common.ErrEmptyQueue)
					})

					Convey("When consuming an item from the queue", func() {
						pl, err := GetTXPayloadFromQueue(p, devEUI)
						So(err, ShouldBeNil)

						Convey("Then the queue size is still 2", func() {
							count, err := GetTXPayloadQueueSize(p, devEUI)
							So(err, ShouldBeNil)
							So(count, ShouldEqual, 2)
						})

						Convey("Then it equals to struct a", func() {
							So(pl, ShouldResemble, a)
						})

						Convey("When consuming an item from the queue again", func() {
							pl, err := GetTXPayloadFromQueue(p, devEUI)
							So(err, ShouldBeNil)
							Convey("Then it equals to struct a (again)", func() {
								So(pl, ShouldResemble, a)
							})
						})

						Convey("When flushing the queue", func() {
							So(FlushTXPayloadQueue(p, devEUI), ShouldBeNil)

							Convey("Then the queue size is 0", func() {
								count, err := GetTXPayloadQueueSize(p, devEUI)
								So(err, ShouldBeNil)
								So(count, ShouldEqual, 0)
							})
						})

						Convey("Then after 150 ms the queue has expired (both in-process and the queue)", func() {
							time.Sleep(100 * time.Millisecond)
							_, err := GetTXPayloadFromQueue(p, devEUI)
							So(err, ShouldResemble, common.ErrEmptyQueue)
						})

						Convey("After clearing the in-process payload", func() {
							_, err := ClearInProcessTXPayload(p, devEUI)
							So(err, ShouldBeNil)

							Convey("Then the queue size is 1", func() {
								count, err := GetTXPayloadQueueSize(p, devEUI)
								So(err, ShouldBeNil)
								So(count, ShouldEqual, 1)
							})

							Convey("When consuming an item from the queue", func() {
								pl, err := GetTXPayloadFromQueue(p, devEUI)
								So(err, ShouldBeNil)
								Convey("Then it equals to struct b", func() {
									So(pl, ShouldResemble, b)
								})
							})
						})
					})
				})
			})
		})
	})
}
