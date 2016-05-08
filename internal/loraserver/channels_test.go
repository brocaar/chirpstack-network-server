package loraserver

import (
	"testing"

	"github.com/brocaar/loraserver/models"
	. "github.com/smartystreets/goconvey/convey"
)

func TestChannelSetAPI(t *testing.T) {
	conf := getConfig()

	Convey("Given a clean database and an API instance", t, func() {
		db, err := OpenDatabase(conf.PostgresDSN)
		So(err, ShouldBeNil)
		mustResetDB(db)

		ctx := Context{
			DB: db,
		}
		api := NewChannelSetAPI(ctx)

		cs := models.ChannelSet{
			Name: "test set",
		}

		Convey("When calling Create", func() {
			So(api.Create(cs, &cs.ID), ShouldBeNil)

			Convey("Then the channel-set has been created", func() {
				var cs2 models.ChannelSet
				So(api.Get(cs.ID, &cs2), ShouldBeNil)
				So(cs2, ShouldResemble, cs)

				Convey("Then the channel-set can be updated", func() {
					cs.Name = "test set 2"
					So(api.Update(cs, &cs.ID), ShouldBeNil)
					So(api.Get(cs.ID, &cs2), ShouldBeNil)
					So(cs2, ShouldResemble, cs)
				})

				Convey("Then the channel-set can be deleted", func() {
					So(api.Delete(cs.ID, &cs.ID), ShouldBeNil)
					So(api.Get(cs.ID, &cs2), ShouldNotBeNil)
				})
			})

			Convey("Then the list of channel-sets has size 1", func() {
				var list []models.ChannelSet
				So(api.GetList(models.GetListRequest{
					Limit:  10,
					Offset: 0,
				}, &list), ShouldBeNil)
				So(list, ShouldHaveLength, 1)
			})
		})
	})
}
