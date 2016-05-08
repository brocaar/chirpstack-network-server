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

func TestChannelAPI(t *testing.T) {
	conf := getConfig()

	Convey("Given a clean database and an API instance", t, func() {
		db, err := OpenDatabase(conf.PostgresDSN)
		So(err, ShouldBeNil)
		mustResetDB(db)

		ctx := Context{
			DB: db,
		}
		api := NewChannelAPI(ctx)

		Convey("Given a channel-set in the database", func() {
			cs := models.ChannelSet{
				Name: "test set",
			}
			So(createChannelSet(db, &cs), ShouldBeNil)

			c1 := models.Channel{
				ChannelSetID: cs.ID,
				Channel:      0,
				Frequency:    868100000,
			}
			c2 := models.Channel{
				ChannelSetID: cs.ID,
				Channel:      1,
				Frequency:    868200000,
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

						Convey("Then GetForChannelSet returns two channels", func() {
							var channels []models.Channel
							So(api.GetForChannelSet(cs.ID, &channels), ShouldBeNil)
							So(channels, ShouldHaveLength, 2)
						})
					})
				})
			})
		})
	})
}
