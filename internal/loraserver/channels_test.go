package loraserver

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/loraserver/models"
)

func TestChannelSetAPI(t *testing.T) {
	conf := common.GetTestConfig()

	Convey("Given a clean database and an API instance", t, func() {
		db, err := storage.OpenDatabase(conf.PostgresDSN)
		So(err, ShouldBeNil)
		common.MustResetDB(db)

		ctx := Context{
			DB: db,
		}
		api := NewChannelListAPI(ctx)

		cl := models.ChannelList{
			Name: "test set",
		}

		Convey("When calling Create", func() {
			So(api.Create(cl, &cl.ID), ShouldBeNil)

			Convey("Then the channel-list has been created", func() {
				var cl2 models.ChannelList
				So(api.Get(cl.ID, &cl2), ShouldBeNil)
				So(cl2, ShouldResemble, cl)

				Convey("Then the channel-list can be updated", func() {
					cl.Name = "test set 2"
					So(api.Update(cl, &cl.ID), ShouldBeNil)
					So(api.Get(cl.ID, &cl2), ShouldBeNil)
					So(cl2, ShouldResemble, cl)
				})

				Convey("Then the channel-list can be deleted", func() {
					So(api.Delete(cl.ID, &cl.ID), ShouldBeNil)
					So(api.Get(cl.ID, &cl2), ShouldNotBeNil)
				})
			})

			Convey("Then the list of channel-list has size 1", func() {
				var list []models.ChannelList
				So(api.GetList(models.GetListRequest{
					Limit:  10,
					Offset: 0,
				}, &list), ShouldBeNil)
				So(list, ShouldHaveLength, 1)
			})
		})
	})
}

func TestChannelAPI(t *testing.T) {
	conf := common.GetTestConfig()

	Convey("Given a clean database and an API instance", t, func() {
		db, err := storage.OpenDatabase(conf.PostgresDSN)
		So(err, ShouldBeNil)
		common.MustResetDB(db)

		ctx := Context{
			DB: db,
		}
		api := NewChannelAPI(ctx)

		Convey("Given a channel-list in the database", func() {
			cl := models.ChannelList{
				Name: "test set",
			}
			So(createChannelList(db, &cl), ShouldBeNil)

			c1 := models.Channel{
				ChannelListID: cl.ID,
				Channel:       3,
				Frequency:     868100000,
			}
			c2 := models.Channel{
				ChannelListID: cl.ID,
				Channel:       5,
				Frequency:     868200000,
			}

			Convey("When calling Create", func() {
				So(api.Create(c1, &c1.ID), ShouldBeNil)

				Convey("Then the channel has been created", func() {
					var c models.Channel
					So(api.Get(c1.ID, &c), ShouldBeNil)
					So(c, ShouldResemble, c1)

					Convey("Then the channel can be updated", func() {
						c1.Frequency = 868300000
						So(api.Update(c1, &c1.ID), ShouldBeNil)
						So(api.Get(c1.ID, &c), ShouldBeNil)
						So(c, ShouldResemble, c1)
					})

					Convey("Then the channel can be deleted", func() {
						So(api.Delete(c1.ID, &c1.ID), ShouldBeNil)
						So(api.Get(c1.ID, &c), ShouldNotBeNil)
					})

					Convey("When creating a second channel", func() {
						So(api.Create(c2, &c2.ID), ShouldBeNil)

						Convey("Then GetForChannelList returns two channels", func() {
							var channels []models.Channel
							So(api.GetForChannelList(cl.ID, &channels), ShouldBeNil)
							So(channels, ShouldHaveLength, 2)
						})
					})
				})
			})
		})
	})
}
