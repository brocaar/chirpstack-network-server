package storage

import (
	"testing"

	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/models"
	. "github.com/smartystreets/goconvey/convey"
)

func TestApplicationMethods(t *testing.T) {
	conf := common.GetTestConfig()

	Convey("Given a clean database", t, func() {
		db, err := OpenDatabase(conf.PostgresDSN)
		So(err, ShouldBeNil)
		common.MustResetDB(db)

		Convey("When creating an application", func() {
			app := models.Application{
				AppEUI: [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
				Name:   "test app",
			}
			So(CreateApplication(db, app), ShouldBeNil)

			Convey("Then we can get it", func() {
				app2, err := GetApplication(db, app.AppEUI)
				So(err, ShouldBeNil)
				So(app2, ShouldResemble, app)
			})

			Convey("Then get applications returns a single item", func() {
				apps, err := GetApplications(db, 10, 0)
				So(err, ShouldBeNil)
				So(apps, ShouldHaveLength, 1)
				So(apps[0], ShouldResemble, app)
			})

			Convey("Then applications count returns 1", func() {
				count, err := GetApplicationsCount(db)
				So(err, ShouldBeNil)
				So(count, ShouldEqual, 1)
			})

			Convey("When updating the application", func() {
				app.Name = "new name"
				So(UpdateApplication(db, app), ShouldBeNil)

				Convey("The application has been updated", func() {
					app2, err := GetApplication(db, app.AppEUI)
					So(err, ShouldBeNil)
					So(app2, ShouldResemble, app)
				})
			})

			Convey("After deleting the application", func() {
				So(DeleteApplication(db, app.AppEUI), ShouldBeNil)

				Convey("The application has gone", func() {
					count, err := GetApplicationsCount(db)
					So(err, ShouldBeNil)
					So(count, ShouldEqual, 0)
				})
			})
		})
	})
}
