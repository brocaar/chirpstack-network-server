package api

import (
	"testing"

	"golang.org/x/net/context"

	pb "github.com/brocaar/loraserver/api"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/loraserver"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/loraserver/models"
	. "github.com/smartystreets/goconvey/convey"
)

func TestNodeAPI(t *testing.T) {
	conf := common.GetTestConfig()

	Convey("Given a clean database with an application and api instance", t, func() {
		db, err := storage.OpenDatabase(conf.PostgresDSN)
		So(err, ShouldBeNil)
		common.MustResetDB(db)
		p := storage.NewRedisPool(conf.RedisURL)
		common.MustFlushRedis(p)

		ctx := context.Background()
		lsCtx := loraserver.Context{DB: db, RedisPool: p}
		api := NewNodeAPI(lsCtx)

		app := models.Application{
			AppEUI: [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
			Name:   "test app",
		}
		So(storage.CreateApplication(db, app), ShouldBeNil)

		Convey("When creating a node", func() {
			_, err := api.Create(ctx, &pb.CreateNodeRequest{
				DevEUI:      "0807060504030201",
				AppEUI:      "0102030405060708",
				AppKey:      "01020304050607080102030405060708",
				RxDelay:     1,
				Rx1DROffset: 3,
			})
			So(err, ShouldBeNil)

			Convey("The node has been created", func() {
				node, err := api.Get(ctx, &pb.GetNodeRequest{DevEUI: "0807060504030201"})
				So(err, ShouldBeNil)
				So(node, ShouldResemble, &pb.GetNodeResponse{
					DevEUI:      "0807060504030201",
					AppEUI:      "0102030405060708",
					AppKey:      "01020304050607080102030405060708",
					RxDelay:     1,
					Rx1DROffset: 3,
				})
			})

			Convey("Then listing the nodes returns a single items", func() {
				nodes, err := api.List(ctx, &pb.ListNodeRequest{
					Limit: 10,
				})
				So(err, ShouldBeNil)
				So(nodes.Result, ShouldHaveLength, 1)
				So(nodes.TotalCount, ShouldEqual, 1)
				So(nodes.Result[0], ShouldResemble, &pb.GetNodeResponse{
					DevEUI:      "0807060504030201",
					AppEUI:      "0102030405060708",
					AppKey:      "01020304050607080102030405060708",
					RxDelay:     1,
					Rx1DROffset: 3,
				})
			})

			Convey("Then listing the nodes for a given AppEUI returns a single item", func() {
				nodes, err := api.ListByAppEUI(ctx, &pb.ListNodeByAppEUIRequest{
					Limit:  10,
					AppEUI: "0102030405060708",
				})
				So(err, ShouldBeNil)
				So(nodes.Result, ShouldHaveLength, 1)
				So(nodes.TotalCount, ShouldEqual, 1)
				So(nodes.Result[0], ShouldResemble, &pb.GetNodeResponse{
					DevEUI:      "0807060504030201",
					AppEUI:      "0102030405060708",
					AppKey:      "01020304050607080102030405060708",
					RxDelay:     1,
					Rx1DROffset: 3,
				})
			})

			Convey("When updating the node", func() {
				_, err := api.Update(ctx, &pb.UpdateNodeRequest{
					DevEUI:      "0807060504030201",
					AppEUI:      "0102030405060708",
					AppKey:      "08070605040302010807060504030201",
					RxDelay:     3,
					Rx1DROffset: 1,
				})
				So(err, ShouldBeNil)

				Convey("Then the node has been updated", func() {
					node, err := api.Get(ctx, &pb.GetNodeRequest{DevEUI: "0807060504030201"})
					So(err, ShouldBeNil)
					So(node, ShouldResemble, &pb.GetNodeResponse{
						DevEUI:      "0807060504030201",
						AppEUI:      "0102030405060708",
						AppKey:      "08070605040302010807060504030201",
						RxDelay:     3,
						Rx1DROffset: 1,
					})
				})
			})

			Convey("After deleting the node", func() {
				_, err := api.Delete(ctx, &pb.DeleteNodeRequest{DevEUI: "0807060504030201"})
				So(err, ShouldBeNil)

				Convey("Then listing the nodes returns zero nodes", func() {
					nodes, err := api.List(ctx, &pb.ListNodeRequest{Limit: 10})
					So(err, ShouldBeNil)
					So(nodes.TotalCount, ShouldEqual, 0)
					So(nodes.Result, ShouldHaveLength, 0)
				})
			})

			Convey("Given a tx payload in the the queue", func() {
				So(storage.AddTXPayloadToQueue(p, models.TXPayload{
					DevEUI: [8]byte{8, 7, 6, 5, 4, 3, 2, 1},
					Data:   []byte("hello!"),
				}), ShouldBeNil)
				count, err := storage.GetTXPayloadQueueSize(p, [8]byte{8, 7, 6, 5, 4, 3, 2, 1})
				So(err, ShouldBeNil)
				So(count, ShouldEqual, 1)

				Convey("When flushing the tx-payload queue", func() {
					_, err := api.FlushTXPayloadQueue(ctx, &pb.FlushTXPayloadQueueRequest{DevEUI: "0807060504030201"})
					So(err, ShouldBeNil)

					Convey("Then the queue is empty", func() {
						count, err := storage.GetTXPayloadQueueSize(p, [8]byte{8, 7, 6, 5, 4, 3, 2, 1})
						So(err, ShouldBeNil)
						So(count, ShouldEqual, 0)
					})
				})
			})
		})
	})
}
