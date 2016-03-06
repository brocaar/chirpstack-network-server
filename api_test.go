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
	})
}
