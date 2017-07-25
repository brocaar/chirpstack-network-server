package api

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/api/ns"
	"github.com/brocaar/loraserver/internal/api/auth"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/gateway"
	"github.com/brocaar/loraserver/internal/maccommand"
	"github.com/brocaar/loraserver/internal/node"
	"github.com/brocaar/loraserver/internal/session"
	"github.com/brocaar/loraserver/internal/test"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/band"
	jwt "github.com/dgrijalva/jwt-go"
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

				Convey("Then the enabled channels are set on the node-session", func() {
					ns, err := session.GetNodeSession(lsCtx.RedisPool, devEUI)
					So(err, ShouldBeNil)
					So(ns.EnabledChannels, ShouldResemble, common.Band.GetUplinkChannels())
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

				Convey("Then the internal fields are still set", func() {
					ns, err := session.GetNodeSession(lsCtx.RedisPool, devEUI)
					So(err, ShouldBeNil)
					So(ns.EnabledChannels, ShouldResemble, common.Band.GetUplinkChannels())
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

			Convey("When enqueueing a downlink mac-command", func() {
				mac := lorawan.MACCommand{
					CID: lorawan.RXParamSetupReq,
					Payload: &lorawan.RX2SetupReqPayload{
						Frequency: 868100000,
					},
				}
				b, err := mac.MarshalBinary()
				So(err, ShouldBeNil)

				_, err = api.EnqueueDataDownMACCommand(ctx, &ns.EnqueueDataDownMACCommandRequest{
					DevEUI:     devEUI[:],
					FrmPayload: true,
					Cid:        uint32(lorawan.RXParamSetupReq),
					Commands:   [][]byte{b},
				})
				So(err, ShouldBeNil)

				Convey("Then the mac-command has been added to the queue", func() {
					queue, err := maccommand.ReadQueueItems(p, devEUI)
					So(err, ShouldBeNil)
					So(queue, ShouldResemble, []maccommand.Block{
						{
							CID:         lorawan.RXParamSetupReq,
							FRMPayload:  true,
							External:    true,
							MACCommands: []lorawan.MACCommand{mac},
						},
					})
				})
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

				Convey("When generating a gateway token", func() {
					common.GatewayServerJWTSecret = "verysecret"

					tokenResp, err := api.GenerateGatewayToken(ctx, &ns.GenerateGatewayTokenRequest{
						Mac: []byte{1, 2, 3, 4, 5, 6, 7, 8},
					})
					So(err, ShouldBeNil)

					Convey("Then a valid JWT token has been returned", func() {
						token, err := jwt.ParseWithClaims(tokenResp.Token, &auth.Claims{}, func(token *jwt.Token) (interface{}, error) {
							if token.Header["alg"] != "HS256" {
								return nil, fmt.Errorf("invalid algorithm %s", token.Header["alg"])
							}
							return []byte("verysecret"), nil
						})
						So(err, ShouldBeNil)
						So(token.Valid, ShouldBeTrue)
						claims, ok := token.Claims.(*auth.Claims)
						So(ok, ShouldBeTrue)
						So(claims.MAC, ShouldEqual, lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8})
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

				Convey("Given 20 logs for two different DevEUIs", func() {
					now := time.Now()
					rxInfoSet := []gw.RXInfo{
						{
							MAC:       lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
							Time:      now,
							Timestamp: 1234,
							Frequency: 868100000,
							Channel:   1,
							RFChain:   1,
							CRCStatus: 1,
							CodeRate:  "4/5",
							RSSI:      110,
							LoRaSNR:   5.5,
							Size:      10,
							DataRate: band.DataRate{
								Modulation:   band.LoRaModulation,
								SpreadFactor: 12,
								Bandwidth:    125,
							},
						},
					}
					txInfo := gw.TXInfo{
						MAC:         lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
						Immediately: true,
						Timestamp:   12345,
						Frequency:   868100000,
						Power:       14,
						CodeRate:    "4/5",
						DataRate: band.DataRate{
							Modulation:   band.LoRaModulation,
							SpreadFactor: 12,
							Bandwidth:    125,
						},
					}
					devEUI1 := lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}
					devEUI2 := lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1}
					phy := lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: lorawan.DevAddr{1, 2, 3, 4},
								FCnt:    1,
							},
						},
					}

					rxBytes, err := json.Marshal(rxInfoSet)
					So(err, ShouldBeNil)
					txBytes, err := json.Marshal(txInfo)
					So(err, ShouldBeNil)
					phyBytes, err := phy.MarshalBinary()
					So(err, ShouldBeNil)

					for i := 0; i < 10; i++ {
						frameLog := node.FrameLog{
							DevEUI:     devEUI1,
							RXInfoSet:  &rxBytes,
							TXInfo:     &txBytes,
							PHYPayload: phyBytes,
						}
						So(node.CreateFrameLog(db, &frameLog), ShouldBeNil)

						frameLog.DevEUI = devEUI2
						frameLog.TXInfo = nil
						So(node.CreateFrameLog(db, &frameLog), ShouldBeNil)
					}

					Convey("Then GetFrameLogsForDevEUI returns the expected result", func() {
						resp, err := api.GetFrameLogsForDevEUI(ctx, &ns.GetFrameLogsForDevEUIRequest{
							DevEUI: devEUI1[:],
							Limit:  1,
							Offset: 0,
						})
						So(err, ShouldBeNil)
						So(resp.TotalCount, ShouldEqual, 10)
						So(resp.Result, ShouldHaveLength, 1)
						So(resp.Result[0].CreatedAt, ShouldNotEqual, "")
						resp.Result[0].CreatedAt = ""
						So(resp.Result[0], ShouldResemble, &ns.FrameLog{
							PhyPayload: phyBytes,
							TxInfo: &ns.TXInfo{
								CodeRate:    "4/5",
								Frequency:   868100000,
								Immediately: true,
								Mac:         []byte{1, 2, 3, 4, 5, 6, 7, 8},
								Power:       14,
								Timestamp:   12345,
								DataRate: &ns.DataRate{
									Modulation:   "LORA",
									BandWidth:    125,
									SpreadFactor: 12,
								},
							},
							RxInfoSet: []*ns.RXInfo{
								{
									Channel:   1,
									CodeRate:  "4/5",
									Frequency: 868100000,
									LoRaSNR:   5.5,
									Rssi:      110,
									Time:      now.Format(time.RFC3339Nano),
									Timestamp: 1234,
									Mac:       []byte{1, 2, 3, 4, 5, 6, 7, 8},
									DataRate: &ns.DataRate{
										Modulation:   "LORA",
										BandWidth:    125,
										SpreadFactor: 12,
									},
								},
							},
						})
					})

					Convey("When creating a channel-configuration", func() {
						cfResp, err := api.CreateChannelConfiguration(ctx, &ns.CreateChannelConfigurationRequest{
							Name:     "test-config",
							Channels: []int32{0, 1, 2},
						})
						So(err, ShouldBeNil)
						So(cfResp.Id, ShouldNotEqual, 0)

						Convey("Then the channel-configuration has been created", func() {
							cf, err := api.GetChannelConfiguration(ctx, &ns.GetChannelConfigurationRequest{
								Id: cfResp.Id,
							})
							So(err, ShouldBeNil)
							So(cf.Name, ShouldEqual, "test-config")
							So(cf.Channels, ShouldResemble, []int32{0, 1, 2})
							So(cf.CreatedAt, ShouldNotEqual, "")
							So(cf.UpdatedAt, ShouldNotEqual, "")

							Convey("Then the channel-configuration can be listed", func() {
								cfs, err := api.ListChannelConfigurations(ctx, &ns.ListChannelConfigurationsRequest{})
								So(err, ShouldBeNil)
								So(cfs.Result, ShouldHaveLength, 1)
								So(cfs.Result[0], ShouldResemble, cf)
							})

							Convey("Then the channel-configuration can be updated", func() {
								_, err := api.UpdateChannelConfiguration(ctx, &ns.UpdateChannelConfigurationRequest{
									Id:       cfResp.Id,
									Name:     "updated-channel-conf",
									Channels: []int32{0, 1},
								})
								So(err, ShouldBeNil)

								cf2, err := api.GetChannelConfiguration(ctx, &ns.GetChannelConfigurationRequest{
									Id: cfResp.Id,
								})
								So(err, ShouldBeNil)
								So(cf2.Name, ShouldEqual, "updated-channel-conf")
								So(cf2.Channels, ShouldResemble, []int32{0, 1})
								So(cf2.CreatedAt, ShouldEqual, cf.CreatedAt)
								So(cf2.UpdatedAt, ShouldNotEqual, "")
								So(cf2.UpdatedAt, ShouldNotEqual, cf.UpdatedAt)
							})
						})

						Convey("Then the channel-configuration can be assigned to the gateway", func() {
							req := ns.UpdateGatewayRequest{
								Mac:                    []byte{1, 2, 3, 4, 5, 6, 7, 8},
								Name:                   "test-gateway-updated",
								Description:            "garden gateway",
								Latitude:               1.1235,
								Longitude:              1.1236,
								Altitude:               15.7,
								ChannelConfigurationID: cfResp.Id,
							}
							_, err := api.UpdateGateway(ctx, &req)
							So(err, ShouldBeNil)

							gw, err := api.GetGateway(ctx, &ns.GetGatewayRequest{
								Mac: []byte{1, 2, 3, 4, 5, 6, 7, 8},
							})
							So(err, ShouldBeNil)
							So(gw.ChannelConfigurationID, ShouldEqual, cfResp.Id)
						})

						Convey("Then the channel-configuration can be deleted", func() {
							_, err := api.DeleteChannelConfiguration(ctx, &ns.DeleteChannelConfigurationRequest{
								Id: cfResp.Id,
							})
							So(err, ShouldBeNil)
							_, err = api.GetChannelConfiguration(ctx, &ns.GetChannelConfigurationRequest{
								Id: cfResp.Id,
							})
							So(err, ShouldNotBeNil)
							So(grpc.Code(err), ShouldEqual, codes.NotFound)
						})

						Convey("Then an extra channel can be added to this configuration", func() {
							ecRes, err := api.CreateExtraChannel(ctx, &ns.CreateExtraChannelRequest{
								ChannelConfigurationID: cfResp.Id,
								Modulation:             ns.Modulation_LORA,
								Frequency:              867100000,
								BandWidth:              125,
								SpreadFactors:          []int32{0, 1, 2, 3, 4, 5},
							})
							So(err, ShouldBeNil)
							So(ecRes.Id, ShouldNotEqual, 0)

							Convey("Then this extra channel can be updated", func() {
								_, err := api.UpdateExtraChannel(ctx, &ns.UpdateExtraChannelRequest{
									Id: ecRes.Id,
									ChannelConfigurationID: cfResp.Id,
									Modulation:             ns.Modulation_LORA,
									Frequency:              867300000,
									BandWidth:              250,
									SpreadFactors:          []int32{5},
								})
								So(err, ShouldBeNil)

								extraChans, err := api.GetExtraChannelsForChannelConfigurationID(ctx, &ns.GetExtraChannelsForChannelConfigurationIDRequest{
									Id: cfResp.Id,
								})
								So(err, ShouldBeNil)
								So(extraChans.Result, ShouldHaveLength, 1)
								So(extraChans.Result[0].Modulation, ShouldEqual, ns.Modulation_LORA)
								So(extraChans.Result[0].Frequency, ShouldEqual, 867300000)
								So(extraChans.Result[0].Bandwidth, ShouldEqual, 250)
								So(extraChans.Result[0].SpreadFactors, ShouldResemble, []int32{5})
								So(extraChans.Result[0].CreatedAt, ShouldNotEqual, "")
								So(extraChans.Result[0].UpdatedAt, ShouldNotEqual, "")
							})

							Convey("Then this extra channel can be removed", func() {
								_, err := api.DeleteExtraChannel(ctx, &ns.DeleteExtraChannelRequest{
									Id: ecRes.Id,
								})
								So(err, ShouldBeNil)

								extraChans, err := api.GetExtraChannelsForChannelConfigurationID(ctx, &ns.GetExtraChannelsForChannelConfigurationIDRequest{
									Id: cfResp.Id,
								})
								So(err, ShouldBeNil)
								So(extraChans.Result, ShouldHaveLength, 0)
							})
						})
					})
				})
			})
		})
	})
}
