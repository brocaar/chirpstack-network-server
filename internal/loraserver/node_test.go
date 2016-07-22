package loraserver

import (
	"testing"
	"time"

	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/loraserver/models"
	"github.com/brocaar/lorawan"
	. "github.com/smartystreets/goconvey/convey"
)

func TestValidateDevNonce(t *testing.T) {
	Convey("Given a Node", t, func() {
		n := models.Node{
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
				_, err := getTXPayloadFromQueue(p, devEUI)
				Convey("Then errEmptyQueue error is returned", func() {
					So(err, ShouldResemble, errEmptyQueue)
				})
			})

			Convey("Given the NodePayloadQueueTTL is 100 ms", func() {
				NodeTXPayloadQueueTTL = 100 * time.Millisecond

				Convey("Given struct a and b are pushed to the queue", func() {
					So(addTXPayloadToQueue(p, a), ShouldBeNil)
					So(addTXPayloadToQueue(p, b), ShouldBeNil)

					Convey("Then the queue size is 2", func() {
						count, err := getTXPayloadQueueSize(p, devEUI)
						So(err, ShouldBeNil)
						So(count, ShouldEqual, 2)
					})

					Convey("Then after 150 ms the queue has expired", func() {
						time.Sleep(150 * time.Millisecond)
						_, err := getTXPayloadFromQueue(p, devEUI)
						So(err, ShouldResemble, errEmptyQueue)
					})

					Convey("When consuming an item from the queue", func() {
						pl, err := getTXPayloadFromQueue(p, devEUI)
						So(err, ShouldBeNil)

						Convey("Then the queue size is still 2", func() {
							count, err := getTXPayloadQueueSize(p, devEUI)
							So(err, ShouldBeNil)
							So(count, ShouldEqual, 2)
						})

						Convey("Then it equals to struct a", func() {
							So(pl, ShouldResemble, a)
						})

						Convey("When consuming an item from the queue again", func() {
							pl, err := getTXPayloadFromQueue(p, devEUI)
							So(err, ShouldBeNil)
							Convey("Then it equals to struct a (again)", func() {
								So(pl, ShouldResemble, a)
							})
						})

						Convey("When flushing the queue", func() {
							So(flushTXPayloadQueue(p, devEUI), ShouldBeNil)

							Convey("Then the queue size is 0", func() {
								count, err := getTXPayloadQueueSize(p, devEUI)
								So(err, ShouldBeNil)
								So(count, ShouldEqual, 0)
							})
						})

						Convey("Then after 150 ms the queue has expired (both in-process and the queue)", func() {
							time.Sleep(100 * time.Millisecond)
							_, err := getTXPayloadFromQueue(p, devEUI)
							So(err, ShouldResemble, errEmptyQueue)
						})

						Convey("After clearing the in-process payload", func() {
							_, err := clearInProcessTXPayload(p, devEUI)
							So(err, ShouldBeNil)

							Convey("Then the queue size is 1", func() {
								count, err := getTXPayloadQueueSize(p, devEUI)
								So(err, ShouldBeNil)
								So(count, ShouldEqual, 1)
							})

							Convey("When consuming an item from the queue", func() {
								pl, err := getTXPayloadFromQueue(p, devEUI)
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

func TestGetCFListForNode(t *testing.T) {
	conf := common.GetTestConfig()

	Convey("Given an application, node (without channel-list) and channel-list with 2 channels", t, func() {
		db, err := OpenDatabase(conf.PostgresDSN)
		So(err, ShouldBeNil)
		common.MustResetDB(db)

		ctx := Context{
			DB: db,
		}

		channelList := models.ChannelList{
			Name: "test channels",
		}
		So(createChannelList(ctx.DB, &channelList), ShouldBeNil)
		So(createChannel(ctx.DB, &models.Channel{
			ChannelListID: channelList.ID,
			Channel:       3,
			Frequency:     868400000,
		}), ShouldBeNil)
		So(createChannel(ctx.DB, &models.Channel{
			ChannelListID: channelList.ID,
			Channel:       4,
			Frequency:     868500000,
		}), ShouldBeNil)

		app := models.Application{
			AppEUI: [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
			Name:   "test app",
		}
		So(storage.CreateApplication(ctx.DB, app), ShouldBeNil)

		node := models.Node{
			DevEUI: [8]byte{8, 7, 6, 5, 4, 3, 2, 1},
			AppEUI: app.AppEUI,
		}
		So(createNode(ctx.DB, node), ShouldBeNil)

		Convey("Then getCFListForNode returns nil", func() {
			cFList, err := getCFListForNode(ctx.DB, node)
			So(err, ShouldBeNil)
			So(cFList, ShouldBeNil)
		})

		Convey("Given the node has the channel-list configured", func() {
			node.ChannelListID = &channelList.ID
			So(updateNode(ctx.DB, node), ShouldBeNil)

			Convey("Then getCFListForNode returns the CFList with the configured channels", func() {
				cFList, err := getCFListForNode(ctx.DB, node)
				So(err, ShouldBeNil)

				So(cFList, ShouldResemble, &lorawan.CFList{
					868400000,
					868500000,
					0,
					0,
					0,
				})
			})

			Convey("Given the configured ISM band does not allow a CFList", func() {
				defer func() {
					Band.ImplementsCFlist = true
				}()
				Band.ImplementsCFlist = false

				Convey("Then getCFListForNode returns nil", func() {
					cFList, err := getCFListForNode(ctx.DB, node)
					So(err, ShouldBeNil)
					So(cFList, ShouldBeNil)
				})
			})
		})

	})
}

func TestNodeAPI(t *testing.T) {
	conf := common.GetTestConfig()

	Convey("Given a clean database and an API instance", t, func() {
		db, err := OpenDatabase(conf.PostgresDSN)
		So(err, ShouldBeNil)
		common.MustResetDB(db)

		ctx := Context{
			DB: db,
		}

		api := NewNodeAPI(ctx)

		Convey("Given an application is created (fk constraint)", func() {
			app := models.Application{
				AppEUI: [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
				Name:   "test app",
			}
			// we need to create the app since the node has a fk constraint
			So(storage.CreateApplication(ctx.DB, app), ShouldBeNil)

			node := models.Node{
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
					var node2 models.Node
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
					var nodes []models.Node
					So(api.GetList(models.GetListRequest{
						Limit:  10,
						Offset: 0,
					}, &nodes), ShouldBeNil)
					So(nodes, ShouldHaveLength, 1)

					So(api.GetListForAppEUI(models.GetListForAppEUIRequest{
						AppEUI: node.AppEUI,
						Limit:  10,
						Offset: 0,
					}, &nodes), ShouldBeNil)
					So(nodes, ShouldHaveLength, 1)
				})
			})
		})
	})
}
