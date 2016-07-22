package api

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"

	pb "github.com/brocaar/loraserver/api"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/loraserver"
)

func TestApplicationAPI(t *testing.T) {
	conf := common.GetTestConfig()

	Convey("Given a clean database and api instance", t, func() {
		db, err := loraserver.OpenDatabase(conf.PostgresDSN)
		So(err, ShouldBeNil)
		common.MustResetDB(db)

		ctx := context.Background()
		lsCtx := loraserver.Context{DB: db}

		api := NewApplicationAPI(lsCtx)

		Convey("When creating an application", func() {
			_, err := api.Create(ctx, &pb.CreateApplicationRequest{AppEUI: "0102030405060708", Name: "test app"})
			So(err, ShouldBeNil)

			Convey("Then we can get it", func() {
				resp, err := api.Get(ctx, &pb.GetApplicationRequest{AppEUI: "0102030405060708"})
				So(err, ShouldBeNil)
				So(resp, ShouldResemble, &pb.GetApplicationResponse{AppEUI: "0102030405060708", Name: "test app"})
			})

			Convey("Then listing the applications returns a single item", func() {
				resp, err := api.List(ctx, &pb.ListApplicationRequest{Limit: 10})
				So(err, ShouldBeNil)
				So(resp.Result, ShouldHaveLength, 1)
				So(resp.TotalCount, ShouldEqual, 1)
				So(resp.Result[0], ShouldResemble, &pb.GetApplicationResponse{AppEUI: "0102030405060708", Name: "test app"})
			})

			Convey("When updating the application", func() {
				_, err := api.Update(ctx, &pb.UpdateApplicationRequest{AppEUI: "0102030405060708", Name: "test app 2"})
				So(err, ShouldBeNil)

				Convey("Then the application has been updated", func() {
					resp, err := api.Get(ctx, &pb.GetApplicationRequest{AppEUI: "0102030405060708"})
					So(err, ShouldBeNil)
					So(resp.Name, ShouldEqual, "test app 2")
				})
			})

			Convey("After deleting the application", func() {
				_, err := api.Delete(ctx, &pb.DeleteApplicationRequest{AppEUI: "0102030405060708"})
				So(err, ShouldBeNil)

				Convey("Then listing the applications resturns zero items", func() {
					resp, err := api.List(ctx, &pb.ListApplicationRequest{Limit: 10})
					So(err, ShouldBeNil)
					So(resp.Result, ShouldHaveLength, 0)
				})
			})
		})
	})
}
