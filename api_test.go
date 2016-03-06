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

		ctx := Context{
			DB: db,
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
	})
}
