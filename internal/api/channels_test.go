package api

import (
	"testing"

	pb "github.com/brocaar/loraserver/api"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/loraserver"
	"github.com/brocaar/loraserver/internal/storage"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func TestChannelListAndChannelAPI(t *testing.T) {
	conf := common.GetTestConfig()

	Convey("Given a clean database and api instances", t, func() {
		db, err := storage.OpenDatabase(conf.PostgresDSN)
		So(err, ShouldBeNil)
		common.MustResetDB(db)

		ctx := context.Background()
		lsCtx := loraserver.Context{DB: db}

		cAPI := NewChannelAPI(lsCtx)
		clAPI := NewChannelListAPI(lsCtx)

		Convey("When creating a channel-list", func() {
			resp, err := clAPI.Create(ctx, &pb.CreateChannelListRequest{Name: "test channel-list"})
			So(err, ShouldBeNil)

			clID := resp.Id

			Convey("Then the channel-list has been created", func() {
				cl, err := clAPI.Get(ctx, &pb.GetChannelListRequest{Id: clID})
				So(err, ShouldBeNil)
				So(cl, ShouldResemble, &pb.GetChannelListResponse{Id: clID, Name: "test channel-list"})
			})

			Convey("When updating the channel-list", func() {
				_, err := clAPI.Update(ctx, &pb.UpdateChannelListRequest{Id: clID, Name: "test channel-list changed"})
				So(err, ShouldBeNil)

				Convey("Then the channel-list has been updated", func() {
					cl, err := clAPI.Get(ctx, &pb.GetChannelListRequest{Id: clID})
					So(err, ShouldBeNil)
					So(cl, ShouldResemble, &pb.GetChannelListResponse{Id: clID, Name: "test channel-list changed"})
				})
			})

			Convey("Then listing the channel-lists returns 1 result", func() {
				resp, err := clAPI.List(ctx, &pb.ListChannelListRequest{Limit: 10, Offset: 0})
				So(err, ShouldBeNil)

				So(resp.TotalCount, ShouldEqual, 1)
				So(resp.Result, ShouldHaveLength, 1)
				So(resp.Result[0], ShouldResemble, &pb.GetChannelListResponse{Id: clID, Name: "test channel-list"})
			})

			Convey("When deleting the channel-list", func() {
				_, err := clAPI.Delete(ctx, &pb.DeleteChannelListRequest{Id: clID})
				So(err, ShouldBeNil)

				Convey("Then the channel-list has been deleted", func() {
					resp, err := clAPI.List(ctx, &pb.ListChannelListRequest{Limit: 10, Offset: 0})
					So(err, ShouldBeNil)

					So(resp.TotalCount, ShouldEqual, 0)
				})
			})

			Convey("When creating a channel", func() {
				resp, err := cAPI.Create(ctx, &pb.CreateChannelRequest{
					ChannelListID: clID,
					Channel:       4,
					Frequency:     868700000,
				})
				So(err, ShouldBeNil)
				cID := resp.Id

				Convey("Then the channel has been created", func() {
					resp, err := cAPI.Get(ctx, &pb.GetChannelRequest{Id: cID})
					So(err, ShouldBeNil)
					So(resp, ShouldResemble, &pb.GetChannelResponse{
						Id:            cID,
						ChannelListID: clID,
						Channel:       4,
						Frequency:     868700000,
					})

					Convey("When updating the channel", func() {
						_, err := cAPI.Update(ctx, &pb.UpdateChannelRequest{
							Id:            cID,
							ChannelListID: clID,
							Channel:       5,
							Frequency:     868700000,
						})
						So(err, ShouldBeNil)

						Convey("Then the channel has been updated", func() {
							resp, err := cAPI.Get(ctx, &pb.GetChannelRequest{Id: cID})
							So(err, ShouldBeNil)
							So(resp, ShouldResemble, &pb.GetChannelResponse{
								Id:            cID,
								ChannelListID: clID,
								Channel:       5,
								Frequency:     868700000,
							})
						})
					})

					Convey("When deleting the channel", func() {
						_, err := cAPI.Delete(ctx, &pb.DeleteChannelRequest{Id: cID})
						So(err, ShouldBeNil)

						Convey("Then the channel has been deleted", func() {
							_, err := cAPI.Get(ctx, &pb.GetChannelRequest{Id: cID})
							So(err, ShouldNotBeNil)
						})
					})

					Convey("When getting all channels for the channel-list", func() {
						resp, err := cAPI.ListByChannelList(ctx, &pb.ListChannelsByChannelListRequest{Id: clID})
						So(err, ShouldBeNil)

						Convey("Then it contains the channel", func() {
							So(resp.Result, ShouldHaveLength, 1)
							So(resp.Result[0], ShouldResemble, &pb.GetChannelResponse{
								Id:            cID,
								ChannelListID: clID,
								Channel:       4,
								Frequency:     868700000,
							})
						})
					})
				})
			})
		})
	})
}
