package storage

import (
	"testing"

	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/models"
	. "github.com/smartystreets/goconvey/convey"
)

func TestChannelFunctions(t *testing.T) {
	conf := common.GetTestConfig()

	Convey("Given a clean database", t, func() {
		db, err := OpenDatabase(conf.PostgresDSN)
		So(err, ShouldBeNil)
		common.MustResetDB(db)

		Convey("When creating a channel-list", func() {
			cl := models.ChannelList{
				Name: "test channel-list",
			}
			So(CreateChannelList(db, &cl), ShouldBeNil)

			Convey("Then the channel-list exists", func() {
				cl2, err := GetChannelList(db, cl.ID)
				So(err, ShouldBeNil)
				So(cl2, ShouldResemble, cl)
			})

			Convey("When updating the channel-list", func() {
				cl.Name = "test channel-list changed"
				So(UpdateChannelList(db, cl), ShouldBeNil)

				Convey("Then the channel-list has been updated", func() {
					cl2, err := GetChannelList(db, cl.ID)
					So(err, ShouldBeNil)
					So(cl2, ShouldResemble, cl)
				})
			})

			Convey("Then listing the channel-lists returns 1 result", func() {
				lists, err := GetChannelLists(db, 10, 0)
				So(err, ShouldBeNil)
				So(lists, ShouldHaveLength, 1)
				So(lists[0], ShouldResemble, cl)
			})

			Convey("Then the channel-list count returns 1", func() {
				count, err := GetChannelListsCount(db)
				So(err, ShouldBeNil)
				So(count, ShouldEqual, 1)
			})

			Convey("When deleting the channel-list", func() {
				So(DeleteChannelList(db, cl.ID), ShouldBeNil)

				Convey("Then the channel-list has been removed", func() {
					count, err := GetChannelListsCount(db)
					So(err, ShouldBeNil)
					So(count, ShouldEqual, 0)
				})
			})

			Convey("When creating a channel", func() {
				c := models.Channel{
					ChannelListID: cl.ID,
					Channel:       4,
					Frequency:     868700000,
				}
				So(CreateChannel(db, &c), ShouldBeNil)

				Convey("Then the channel has been created", func() {
					c2, err := GetChannel(db, c.ID)
					So(err, ShouldBeNil)
					So(c2, ShouldResemble, c)
				})

				Convey("When updating the channel", func() {
					c.Channel = 5
					So(UpdateChannel(db, c), ShouldBeNil)

					Convey("Then the channel has been updated", func() {
						c2, err := GetChannel(db, c.ID)
						So(err, ShouldBeNil)
						So(c2, ShouldResemble, c)
					})
				})

				Convey("When deleting the channel", func() {
					So(DeleteChannel(db, c.ID), ShouldBeNil)

					Convey("Then the channel has been deleted", func() {
						_, err := GetChannel(db, c.ID)
						So(err, ShouldNotBeNil)
					})
				})

				Convey("When getting all channels for the channel-list", func() {
					channels, err := GetChannelsForChannelList(db, cl.ID)
					So(err, ShouldBeNil)

					Convey("Then it contains the channel", func() {
						So(channels, ShouldHaveLength, 1)
						So(channels[0], ShouldResemble, c)
					})
				})
			})
		})
	})
}
