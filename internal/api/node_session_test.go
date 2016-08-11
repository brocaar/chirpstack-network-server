package api

import (
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"golang.org/x/net/context"

	pb "github.com/brocaar/loraserver/api"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/loraserver"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/loraserver/models"
	. "github.com/smartystreets/goconvey/convey"
)

func TestNodeSessionAPI(t *testing.T) {
	conf := common.GetTestConfig()

	Convey("Given a clean database and api instance", t, func() {
		db, err := storage.OpenDatabase(conf.PostgresDSN)
		So(err, ShouldBeNil)
		common.MustResetDB(db)

		p := storage.NewRedisPool(conf.RedisURL)
		common.MustFlushRedis(p)

		lsCtx := loraserver.Context{DB: db, RedisPool: p, NetID: [3]byte{1, 2, 3}}
		ctx := context.Background()
		validator := &TestValidator{}
		api := NewNodeSessionAPI(lsCtx, validator)

		Convey("Given an application and node are created (fk constraints)", func() {
			app := models.Application{
				AppEUI: [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
				Name:   "test app",
			}
			So(storage.CreateApplication(db, app), ShouldBeNil)

			node := models.Node{
				DevEUI:        [8]byte{8, 7, 6, 5, 4, 3, 2, 1},
				AppEUI:        [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
				AppKey:        [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
				UsedDevNonces: [][2]byte{},
			}
			So(storage.CreateNode(db, node), ShouldBeNil)

			Convey("When creating a node-session", func() {
				_, err := api.Create(ctx, &pb.CreateNodeSessionRequest{
					DevAddr:     "06020304",
					DevEUI:      node.DevEUI.String(),
					AppEUI:      node.AppEUI.String(),
					AppSKey:     node.AppKey.String(),
					NwkSKey:     node.AppKey.String(),
					FCntUp:      10,
					FCntDown:    11,
					RxDelay:     1,
					Rx1DROffset: 2,
					CFList: []uint32{
						868700000,
					},
				})
				So(err, ShouldBeNil)
				So(validator.ctx, ShouldResemble, ctx)
				So(validator.validatorFuncs, ShouldHaveLength, 3)

				Convey("Then it can be retrieved by DevAddr", func() {
					resp, err := api.Get(ctx, &pb.GetNodeSessionRequest{DevAddr: "06020304"})
					So(err, ShouldBeNil)
					So(validator.ctx, ShouldResemble, ctx)
					So(validator.validatorFuncs, ShouldHaveLength, 3)
					So(resp, ShouldResemble, &pb.GetNodeSessionResponse{
						DevAddr:     "06020304",
						DevEUI:      node.DevEUI.String(),
						AppEUI:      node.AppEUI.String(),
						AppSKey:     node.AppKey.String(),
						NwkSKey:     node.AppKey.String(),
						FCntUp:      10,
						FCntDown:    11,
						RxDelay:     1,
						Rx1DROffset: 2,
						CFList: []uint32{
							868700000,
							0,
							0,
							0,
							0,
						},
					})
				})

				Convey("Then it can be retrieved by DevEUI", func() {
					resp, err := api.GetByDevEUI(ctx, &pb.GetNodeSessionByDevEUIRequest{DevEUI: node.DevEUI.String()})
					So(err, ShouldBeNil)
					So(validator.ctx, ShouldResemble, ctx)
					So(validator.validatorFuncs, ShouldHaveLength, 3)
					So(resp, ShouldResemble, &pb.GetNodeSessionResponse{
						DevAddr:     "06020304",
						DevEUI:      node.DevEUI.String(),
						AppEUI:      node.AppEUI.String(),
						AppSKey:     node.AppKey.String(),
						NwkSKey:     node.AppKey.String(),
						FCntUp:      10,
						FCntDown:    11,
						RxDelay:     1,
						Rx1DROffset: 2,
						CFList: []uint32{
							868700000,
							0,
							0,
							0,
							0,
						},
					})
				})

				Convey("When updating a node-session that belongs to a different DevEUI", func() {
					_, err := api.Update(ctx, &pb.UpdateNodeSessionRequest{
						DevAddr:     "06020304",
						DevEUI:      "0101010101010101",
						AppEUI:      node.AppEUI.String(),
						AppSKey:     node.AppKey.String(),
						NwkSKey:     node.AppKey.String(),
						FCntUp:      20,
						FCntDown:    22,
						RxDelay:     10,
						Rx1DROffset: 20,
						CFList: []uint32{
							868700000,
							868800000,
						},
					})

					Convey("Then an error is returned", func() {
						So(err, ShouldResemble, grpc.Errorf(codes.InvalidArgument, "node-session belongs to a different DevEUI"))
					})
				})

				Convey("When updating a node-session that belongs to a different AppEUI", func() {
					_, err := api.Update(ctx, &pb.UpdateNodeSessionRequest{
						DevAddr:     "06020304",
						DevEUI:      node.DevEUI.String(),
						AppEUI:      "0101010101010101",
						AppSKey:     node.AppKey.String(),
						NwkSKey:     node.AppKey.String(),
						FCntUp:      20,
						FCntDown:    22,
						RxDelay:     10,
						Rx1DROffset: 20,
						CFList: []uint32{
							868700000,
							868800000,
						},
					})
					Convey("Then an error is returned", func() {
						So(err, ShouldResemble, grpc.Errorf(codes.InvalidArgument, "node-session belongs to a different AppEUI"))
					})
				})

				Convey("When updating the node-session", func() {
					_, err := api.Update(ctx, &pb.UpdateNodeSessionRequest{
						DevAddr:     "06020304",
						DevEUI:      node.DevEUI.String(),
						AppEUI:      node.AppEUI.String(),
						AppSKey:     node.AppKey.String(),
						NwkSKey:     node.AppKey.String(),
						FCntUp:      20,
						FCntDown:    22,
						RxDelay:     10,
						Rx1DROffset: 20,
						CFList: []uint32{
							868700000,
							868800000,
						},
					})
					So(err, ShouldBeNil)
					So(validator.ctx, ShouldResemble, ctx)
					So(validator.validatorFuncs, ShouldHaveLength, 3)

					Convey("Then the node-session has been updated", func() {
						resp, err := api.Get(ctx, &pb.GetNodeSessionRequest{DevAddr: "06020304"})
						So(err, ShouldBeNil)
						So(resp, ShouldResemble, &pb.GetNodeSessionResponse{
							DevAddr:     "06020304",
							DevEUI:      node.DevEUI.String(),
							AppEUI:      node.AppEUI.String(),
							AppSKey:     node.AppKey.String(),
							NwkSKey:     node.AppKey.String(),
							FCntUp:      20,
							FCntDown:    22,
							RxDelay:     10,
							Rx1DROffset: 20,
							CFList: []uint32{
								868700000,
								868800000,
								0,
								0,
								0,
							},
						})
					})
				})

				Convey("When deleting the node-session", func() {
					_, err := api.Delete(ctx, &pb.DeleteNodeSessionRequest{DevAddr: "06020304"})
					So(err, ShouldBeNil)
					So(validator.ctx, ShouldResemble, ctx)
					So(validator.validatorFuncs, ShouldHaveLength, 3)

					Convey("Then the node-session has been deleted", func() {
						_, err := api.Get(ctx, &pb.GetNodeSessionRequest{DevAddr: "06020304"})
						So(err, ShouldNotBeNil)
					})
				})

				Convey("When calling GetRandomDevAddr", func() {
					resp, err := api.GetRandomDevAddr(ctx, &pb.GetRandomDevAddrRequest{})
					So(err, ShouldBeNil)
					So(validator.ctx, ShouldResemble, ctx)
					So(validator.validatorFuncs, ShouldHaveLength, 1)
					So(resp.DevAddr, ShouldHaveLength, 8)
				})
			})
		})
	})
}
