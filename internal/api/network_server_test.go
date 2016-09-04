package api

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/brocaar/loraserver/api/ns"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/test"
	. "github.com/smartystreets/goconvey/convey"
)

func TestNetworkServerAPI(t *testing.T) {
	conf := test.GetConfig()

	Convey("Given a clean Redis database + api instance", t, func() {
		p := common.NewRedisPool(conf.RedisURL)
		test.MustFlushRedis(p)

		lsCtx := common.Context{
			RedisPool: p,
			NetID:     [3]byte{1, 2, 3},
		}
		ctx := context.Background()
		api := NetworkServerAPI{ctx: lsCtx}

		devAddr := [4]byte{6, 2, 3, 4}
		devEUI := [8]byte{1, 2, 3, 4, 5, 6, 7, 8}
		appEUI := [8]byte{8, 7, 6, 5, 4, 3, 2, 1}
		nwkSKey := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}

		Convey("When creating a node-session", func() {
			_, err := api.CreateNodeSession(ctx, &ns.CreateNodeSessionRequest{
				DevAddr:     devAddr[:],
				DevEUI:      devEUI[:],
				AppEUI:      appEUI[:],
				NwkSKey:     nwkSKey[:],
				FCntUp:      10,
				FCntDown:    11,
				RxDelay:     1,
				Rx1DROffset: 2,
				CFList: []uint32{
					868700000,
				},
				RxWindow: ns.RXWindow_RX2,
				Rx2DR:    3,
			})
			So(err, ShouldBeNil)

			Convey("Then it can be retrieved by DevEUI", func() {
				resp, err := api.GetNodeSession(ctx, &ns.GetNodeSessionRequest{
					DevEUI: devEUI[:],
				})
				So(err, ShouldBeNil)
				So(resp, ShouldResemble, &ns.GetNodeSessionResponse{
					DevAddr:     devAddr[:],
					DevEUI:      devEUI[:],
					AppEUI:      appEUI[:],
					NwkSKey:     nwkSKey[:],
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
					RxWindow: ns.RXWindow_RX2,
					Rx2DR:    3,
				})
			})

			Convey("When updating a node-session that belongs to a different DevEUI", func() {
				_, err := api.UpdateNodeSession(ctx, &ns.UpdateNodeSessionRequest{
					DevAddr:     devAddr[:],
					DevEUI:      []byte{2, 2, 3, 4, 5, 6, 7, 8},
					AppEUI:      appEUI[:],
					NwkSKey:     nwkSKey[:],
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
				_, err := api.UpdateNodeSession(ctx, &ns.UpdateNodeSessionRequest{
					DevAddr:     devAddr[:],
					DevEUI:      devEUI[:],
					AppEUI:      []byte{8, 7, 6, 5, 4, 3, 2, 2},
					NwkSKey:     nwkSKey[:],
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
				_, err := api.UpdateNodeSession(ctx, &ns.UpdateNodeSessionRequest{
					DevAddr:     devAddr[:],
					DevEUI:      devEUI[:],
					AppEUI:      appEUI[:],
					NwkSKey:     nwkSKey[:],
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

				Convey("Then the node-session has been updated", func() {
					resp, err := api.GetNodeSession(ctx, &ns.GetNodeSessionRequest{
						DevEUI: devEUI[:],
					})
					So(err, ShouldBeNil)
					So(resp, ShouldResemble, &ns.GetNodeSessionResponse{
						DevAddr:     devAddr[:],
						DevEUI:      devEUI[:],
						AppEUI:      appEUI[:],
						NwkSKey:     nwkSKey[:],
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
				_, err := api.DeleteNodeSession(ctx, &ns.DeleteNodeSessionRequest{
					DevEUI: devEUI[:],
				})
				So(err, ShouldBeNil)

				Convey("Then the node-session has been deleted", func() {
					_, err := api.GetNodeSession(ctx, &ns.GetNodeSessionRequest{
						DevEUI: devEUI[:],
					})
					So(err, ShouldNotBeNil)
				})
			})

			Convey("When calling GetRandomDevAddr", func() {
				resp, err := api.GetRandomDevAddr(ctx, &ns.GetRandomDevAddrRequest{})
				So(err, ShouldBeNil)
				So(resp.DevAddr, ShouldHaveLength, 4)
			})
		})
	})
}
