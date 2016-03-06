package loraserver

import (
	"testing"

	"github.com/brocaar/lorawan"
	. "github.com/smartystreets/goconvey/convey"
)

func TestAPI(t *testing.T) {
	conf := getConfig()

	Convey("Given a clean Postgres server", t, func() {
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

		api := NewAPI(ctx)

		Convey("Application methods", func() {
			app := Application{
				AppEUI: [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
				Name:   "test app",
			}

			Convey("When calling CreateApplication", func() {
				var appEUI lorawan.EUI64
				So(api.CreateApplication(app, &appEUI), ShouldBeNil)
				So(appEUI, ShouldEqual, app.AppEUI)

				Convey("Then the application was created", func() {
					var app2 Application
					So(api.GetApplication(app.AppEUI, &app2), ShouldBeNil)
					So(app2, ShouldResemble, app)

					Convey("Then the application can be updated", func() {
						var appEUI lorawan.EUI64
						app.Name = "test app 2"
						So(api.UpdateApplication(app, &appEUI), ShouldBeNil)
						So(api.GetApplication(app.AppEUI, &app2), ShouldBeNil)
						So(app2, ShouldResemble, app)
					})
				})

				Convey("Then the application can be deleted", func() {
					var appEUI lorawan.EUI64
					var app2 Application
					So(api.DeleteApplication(app.AppEUI, &appEUI), ShouldBeNil)
					So(api.GetApplication(app.AppEUI, &app2), ShouldNotBeNil)
				})
			})
		})

		Convey("Node methods", func() {
			app := Application{
				AppEUI: [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
				Name:   "test app",
			}
			// we need to create the app since the node has a fk constraint
			So(CreateApplication(ctx.DB, app), ShouldBeNil)

			node := Node{
				DevEUI:        [8]byte{8, 7, 6, 5, 4, 3, 2, 1},
				AppEUI:        [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
				AppKey:        [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
				UsedDevNonces: [][2]byte{},
			}

			Convey("When calling CreateNode", func() {
				var devEUI lorawan.EUI64
				So(api.CreateNode(node, &devEUI), ShouldBeNil)
				So(devEUI, ShouldEqual, node.DevEUI)

				Convey("Then the node has been created", func() {
					var node2 Node
					So(api.GetNode(node.DevEUI, &node2), ShouldBeNil)
					So(node2, ShouldResemble, node)

					Convey("Then the node can ben updated", func() {
						node.UsedDevNonces = [][2]byte{
							{1, 2},
						}
						So(api.UpdateNode(node, &devEUI), ShouldBeNil)
						So(api.GetNode(node.DevEUI, &node2), ShouldBeNil)
						So(node2, ShouldResemble, node)
					})

					Convey("Then the node can be deleted", func() {
						So(api.DeleteNode(node.DevEUI, &devEUI), ShouldBeNil)
						So(api.GetNode(node.DevEUI, &node2), ShouldNotBeNil)
					})
				})
			})
		})

		Convey("NodeSession methods", func() {
			app := Application{
				AppEUI: [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
				Name:   "test app",
			}
			// we need to create the app since the node has a fk constraint
			So(CreateApplication(ctx.DB, app), ShouldBeNil)

			node := Node{
				DevEUI:        [8]byte{8, 7, 6, 5, 4, 3, 2, 1},
				AppEUI:        [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
				AppKey:        [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
				UsedDevNonces: [][2]byte{},
			}
			So(CreateNode(ctx.DB, node), ShouldBeNil)

			ns := NodeSession{
				DevAddr:  [4]byte{6, 2, 3, 4},
				DevEUI:   node.DevEUI,
				AppEUI:   node.AppEUI,
				AppSKey:  node.AppKey,
				NwkSKey:  node.AppKey,
				FCntUp:   10,
				FCntDown: 11,
			}

			Convey("When calling CreateNodeSession", func() {
				var devAddr lorawan.DevAddr
				So(api.CreateNodeSession(ns, &devAddr), ShouldBeNil)
				So(devAddr, ShouldResemble, ns.DevAddr)

				Convey("Then the session has been created", func() {
					var ns2 NodeSession
					So(api.GetNodeSession(ns.DevAddr, &ns2), ShouldBeNil)
					So(ns2, ShouldResemble, ns)

					Convey("Then the session can be deleted", func() {
						So(api.DeleteNodeSession(ns.DevAddr, &devAddr), ShouldBeNil)
						So(api.GetNodeSession(ns.DevAddr, &ns2), ShouldNotBeNil)
					})
				})
			})

			Convey("When calling CreateNodeSession with a DevAddr which has a wrong NwkID", func() {
				var devAddr lorawan.DevAddr
				ns.DevAddr = [4]byte{1, 2, 3, 4}
				err := api.CreateNodeSession(ns, &devAddr)
				Convey("Then an error is returned", func() {
					So(err, ShouldNotBeNil)
				})
			})
		})
	})
}
