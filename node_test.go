package loraserver

import (
	"testing"
	"time"

	"github.com/brocaar/lorawan"
	. "github.com/smartystreets/goconvey/convey"
)

func TestValidateDevNonce(t *testing.T) {
	Convey("Given a Node", t, func() {
		n := Node{
			UsedDevNonces: make([][2]byte, 0),
		}

		Convey("Then any given dev-nonce is valid", func() {
			dn := [2]byte{1, 2}
			So(n.ValidateDevNonce(dn), ShouldBeTrue)

			Convey("Then the dev-nonce is added to the used nonces", func() {
				So(n.UsedDevNonces, ShouldContain, dn)
			})
		})

		Convey("Given a Node has 10 used nonces", func() {
			n.UsedDevNonces = [][2]byte{
				{1, 1},
				{2, 2},
				{3, 3},
				{4, 4},
				{5, 5},
				{6, 6},
				{7, 7},
				{8, 8},
				{9, 9},
				{10, 10},
			}

			Convey("Then an already used nonce is invalid", func() {
				So(n.ValidateDevNonce([2]byte{1, 1}), ShouldBeFalse)

				Convey("Then the UsedDevNonces still has length 10", func() {
					So(n.UsedDevNonces, ShouldHaveLength, 10)
				})
			})

			Convey("Then a new nonce is valid", func() {
				So(n.ValidateDevNonce([2]byte{0, 0}), ShouldBeTrue)

				Convey("Then the UsedDevNonces still has length 10", func() {
					So(n.UsedDevNonces, ShouldHaveLength, 10)
				})

				Convey("Then the new nonce was added to the back of the list", func() {
					So(n.UsedDevNonces[9], ShouldEqual, [2]byte{0, 0})
				})
			})
		})
	})
}

func TestTXPayloadQueue(t *testing.T) {
	conf := getConfig()

	Convey("Given a clean Redis database", t, func() {
		p := NewRedisPool(conf.RedisURL)
		mustFlushRedis(p)

		Convey("Given two TXPayload structs (a and b) for the same DevEUI", func() {
			devEUI := [8]byte{1, 2, 3, 4, 5, 6, 7, 8}
			a := TXPayload{
				DevEUI: devEUI,
				Data:   []byte("hello!"),
			}
			b := TXPayload{
				DevEUI: devEUI,
				Data:   []byte("world"),
			}

			Convey("When getting an item from the non-existing queue", func() {
				_, _, err := getTXPayloadAndRemainingFromQueue(p, devEUI)
				Convey("Then an errDoesNotExist error is returned", func() {
					So(err, ShouldEqual, errDoesNotExist)
				})
			})

			Convey("Given the NodePayloadQueueTTL is 100 ms", func() {
				NodeTXPayloadQueueTTL = 100 * time.Millisecond

				Convey("Given struct a and b are pushed to the queue", func() {
					So(addTXPayloadToQueue(p, a), ShouldBeNil)
					So(addTXPayloadToQueue(p, b), ShouldBeNil)

					Convey("Then after 150 ms the queue has expired", func() {
						time.Sleep(150 * time.Millisecond)
						_, _, err := getTXPayloadAndRemainingFromQueue(p, devEUI)
						So(err, ShouldEqual, errDoesNotExist)
					})

					Convey("When consuming an item from the queue", func() {
						pl, r, err := getTXPayloadAndRemainingFromQueue(p, devEUI)
						So(err, ShouldBeNil)
						Convey("Then it equals to struct a", func() {
							So(pl, ShouldResemble, a)
						})
						Convey("Then there are remaining items in the queue", func() {
							So(r, ShouldBeTrue)
						})

						Convey("When consuming an item from the queue again", func() {
							pl, r, err := getTXPayloadAndRemainingFromQueue(p, devEUI)
							So(err, ShouldBeNil)
							Convey("Then it equals to struct a (again)", func() {
								So(pl, ShouldResemble, a)
							})
							Convey("Then there are remaining items in the queue", func() {
								So(r, ShouldBeTrue)
							})
						})

						Convey("After clearing the in-process payload", func() {
							So(clearInProcessTXPayload(p, devEUI), ShouldBeNil)

							Convey("When consuming an item from the queue", func() {
								pl, r, err := getTXPayloadAndRemainingFromQueue(p, devEUI)
								So(err, ShouldBeNil)
								Convey("Then it equals to struct b", func() {
									So(pl, ShouldResemble, b)
								})
								Convey("Then there are no remaining items in the queue", func() {
									So(r, ShouldBeFalse)
								})
							})
						})
					})
				})
			})
		})
	})
}

func TestNodeAPI(t *testing.T) {
	conf := getConfig()

	Convey("Given a clean database and an API instance", t, func() {
		db, err := OpenDatabase(conf.PostgresDSN)
		So(err, ShouldBeNil)
		mustResetDB(db)

		ctx := Context{
			DB: db,
		}

		api := NewNodeAPI(ctx)

		Convey("Given an application is created (fk constraint)", func() {
			app := Application{
				AppEUI: [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
				Name:   "test app",
			}
			// we need to create the app since the node has a fk constraint
			So(createApplication(ctx.DB, app), ShouldBeNil)

			node := Node{
				DevEUI:        [8]byte{8, 7, 6, 5, 4, 3, 2, 1},
				AppEUI:        [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
				AppKey:        [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
				UsedDevNonces: [][2]byte{},
			}

			Convey("When calling Create", func() {
				var devEUI lorawan.EUI64
				So(api.Create(node, &devEUI), ShouldBeNil)
				So(devEUI, ShouldEqual, node.DevEUI)

				Convey("Then the node has been created", func() {
					var node2 Node
					So(api.Get(node.DevEUI, &node2), ShouldBeNil)
					So(node2, ShouldResemble, node)

					Convey("Then the node can ben updated", func() {
						node.UsedDevNonces = [][2]byte{
							{1, 2},
						}
						So(api.Update(node, &devEUI), ShouldBeNil)
						So(api.Get(node.DevEUI, &node2), ShouldBeNil)
						So(node2, ShouldResemble, node)
					})

					Convey("Then the node can be deleted", func() {
						So(api.Delete(node.DevEUI, &devEUI), ShouldBeNil)
						So(api.Get(node.DevEUI, &node2), ShouldNotBeNil)
					})
				})

				Convey("Then the list of nodes has size 1", func() {
					var nodes []Node
					So(api.GetList(GetListRequest{
						Limit:  10,
						Offset: 0,
					}, &nodes), ShouldBeNil)
				})
			})
		})
	})
}
