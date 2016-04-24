package loraserver

import (
	"testing"

	"github.com/brocaar/loraserver/models"
	"github.com/brocaar/lorawan"
	. "github.com/smartystreets/goconvey/convey"
)

func TestApplicationAPI(t *testing.T) {
	conf := getConfig()

	Convey("Given a clean database and an API instance", t, func() {
		db, err := OpenDatabase(conf.PostgresDSN)
		So(err, ShouldBeNil)
		mustResetDB(db)
		nodeApplicationsManager, err := NewNodeApplicationsManager(db)
		So(err, ShouldBeNil)

		ctx := Context{
			NodeAppManager: nodeApplicationsManager,
		}

		api := NewApplicationAPI(ctx)

		app := models.Application{
			AppEUI: [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
			Name:   "test app",
		}

		Convey("When calling Create", func() {
			var appEUI lorawan.EUI64
			So(api.Create(app, &appEUI), ShouldBeNil)
			So(appEUI, ShouldEqual, app.AppEUI)

			Convey("Then the application was created", func() {
				var app2 models.Application
				So(api.Get(app.AppEUI, &app2), ShouldBeNil)
				So(app2, ShouldResemble, app)

				Convey("Then the application can be updated", func() {
					var appEUI lorawan.EUI64
					app.Name = "test app 2"
					So(api.Update(app, &appEUI), ShouldBeNil)
					So(api.Get(app.AppEUI, &app2), ShouldBeNil)
					So(app2, ShouldResemble, app)
				})
			})

			Convey("Then the list of applications has size 1", func() {
				var apps []models.Application
				So(api.GetList(models.GetListRequest{
					Limit:  10,
					Offset: 0,
				}, &apps), ShouldBeNil)
				So(apps, ShouldHaveLength, 1)
			})

			Convey("Then the application can be deleted", func() {
				var appEUI lorawan.EUI64
				var app2 models.Application
				So(api.Delete(app.AppEUI, &appEUI), ShouldBeNil)
				So(api.Get(app.AppEUI, &app2), ShouldNotBeNil)
			})
		})
	})
}
