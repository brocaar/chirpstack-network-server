package api

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/brocaar/loraserver/api/ns"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/gateway"
	"github.com/brocaar/loraserver/internal/test"
	. "github.com/smartystreets/goconvey/convey"
)

func TestNetworkServerAPI(t *testing.T) {
	conf := test.GetConfig()

	Convey("Given a clean PostgreSQL and Redis database + api instance", t, func() {
		p := common.NewRedisPool(conf.RedisURL)
		test.MustFlushRedis(p)
		db, err := common.OpenDatabase(conf.PostgresDSN)
		So(err, ShouldBeNil)
		test.MustResetDB(db)

		lsCtx := common.Context{
			RedisPool: p,
			DB:        db,
			NetID:     [3]byte{1, 2, 3},
		}
		ctx := context.Background()
		api := NetworkServerAPI{ctx: lsCtx}

		gateway.MustSetStatsAggregationIntervals([]string{"MINUTE"})

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
				RxWindow:    ns.RXWindow_RX2,
				Rx2DR:       3,
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
					RxWindow:    ns.RXWindow_RX2,
					Rx2DR:       3,
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

			Convey("When creating a gateway", func() {
				req := ns.CreateGatewayRequest{
					Mac:         []byte{1, 2, 3, 4, 5, 6, 7, 8},
					Name:        "test-gateway",
					Description: "rooftop gateway",
					Latitude:    1.1234,
					Longitude:   1.1235,
					Altitude:    15.5,
				}

				_, err := api.CreateGateway(ctx, &req)
				So(err, ShouldBeNil)

				Convey("Then it can be retrieved", func() {
					resp, err := api.GetGateway(ctx, &ns.GetGatewayRequest{Mac: req.Mac})
					So(err, ShouldBeNil)
					So(resp.Mac, ShouldResemble, req.Mac)
					So(resp.Name, ShouldEqual, req.Name)
					So(resp.Description, ShouldEqual, req.Description)
					So(resp.Latitude, ShouldEqual, req.Latitude)
					So(resp.Longitude, ShouldEqual, req.Longitude)
					So(resp.Altitude, ShouldEqual, req.Altitude)
					So(resp.CreatedAt, ShouldNotEqual, "")
					So(resp.UpdatedAt, ShouldNotEqual, "")
					So(resp.FirstSeenAt, ShouldEqual, "")
					So(resp.LastSeenAt, ShouldEqual, "")
				})

				Convey("Then it can be updated", func() {
					req := ns.UpdateGatewayRequest{
						Mac:         []byte{1, 2, 3, 4, 5, 6, 7, 8},
						Name:        "test-gateway-updated",
						Description: "garden gateway",
						Latitude:    1.1235,
						Longitude:   1.1236,
						Altitude:    15.7,
					}
					_, err := api.UpdateGateway(ctx, &req)
					So(err, ShouldBeNil)

					resp, err := api.GetGateway(ctx, &ns.GetGatewayRequest{Mac: req.Mac})
					So(err, ShouldBeNil)
					So(resp.Mac, ShouldResemble, req.Mac)
					So(resp.Name, ShouldEqual, req.Name)
					So(resp.Description, ShouldEqual, req.Description)
					So(resp.Latitude, ShouldEqual, req.Latitude)
					So(resp.Longitude, ShouldEqual, req.Longitude)
					So(resp.Altitude, ShouldEqual, req.Altitude)
					So(resp.CreatedAt, ShouldNotEqual, "")
					So(resp.UpdatedAt, ShouldNotEqual, "")
					So(resp.FirstSeenAt, ShouldEqual, "")
					So(resp.LastSeenAt, ShouldEqual, "")
				})

				Convey("Then listing the gateways returns the gateway", func() {
					resp, err := api.ListGateways(ctx, &ns.ListGatewayRequest{
						Limit:  10,
						Offset: 0,
					})
					So(err, ShouldBeNil)
					So(resp.TotalCount, ShouldEqual, 1)
					So(resp.Result, ShouldHaveLength, 1)
					So(resp.Result[0].Mac, ShouldResemble, []byte{1, 2, 3, 4, 5, 6, 7, 8})
				})

				Convey("When deleting the gateway", func() {
					_, err := api.DeleteGateway(ctx, &ns.DeleteGatewayRequest{
						Mac: []byte{1, 2, 3, 4, 5, 6, 7, 8},
					})
					So(err, ShouldBeNil)

					Convey("Then getting the gateway returns an error", func() {
						_, err := api.GetGateway(ctx, &ns.GetGatewayRequest{
							Mac: []byte{1, 2, 3, 4, 5, 6, 7, 8},
						})
						So(err, ShouldResemble, grpc.Errorf(codes.NotFound, "gateway does not exist"))
					})
				})

				Convey("Given some stats for this gateway", func() {
					now := time.Now().UTC()
					_, err := db.Exec(`
						insert into gateway_stats (
							mac,
							"timestamp",
							"interval",
							rx_packets_received,
							rx_packets_received_ok,
							tx_packets_received,
							tx_packets_emitted
						) values ($1, $2, $3, $4, $5, $6, $7)`,
						[]byte{1, 2, 3, 4, 5, 6, 7, 8},
						now.Truncate(time.Minute),
						"MINUTE",
						10,
						5,
						11,
						10,
					)
					So(err, ShouldBeNil)

					Convey("Listing the gateway stats returns these stats", func() {
						resp, err := api.GetGatewayStats(ctx, &ns.GetGatewayStatsRequest{
							Mac:            []byte{1, 2, 3, 4, 5, 6, 7, 8},
							Interval:       ns.AggregationInterval_MINUTE,
							StartTimestamp: now.Truncate(time.Minute).Format(time.RFC3339Nano),
							EndTimestamp:   now.Format(time.RFC3339Nano),
						})
						So(err, ShouldBeNil)
						So(resp.Result, ShouldHaveLength, 1)
						ts, err := time.Parse(time.RFC3339Nano, resp.Result[0].Timestamp)
						So(err, ShouldBeNil)
						So(ts.Equal(now.Truncate(time.Minute)), ShouldBeTrue)
						So(resp.Result[0].RxPacketsReceived, ShouldEqual, 10)
						So(resp.Result[0].RxPacketsReceivedOK, ShouldEqual, 5)
						So(resp.Result[0].TxPacketsReceived, ShouldEqual, 11)
						So(resp.Result[0].TxPacketsEmitted, ShouldEqual, 10)
					})
				})
			})
		})
	})
}
