package api

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/brocaar/lorawan/band"

	jwt "github.com/dgrijalva/jwt-go"
	. "github.com/smartystreets/goconvey/convey"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/api/ns"
	"github.com/brocaar/loraserver/internal/api/auth"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/framelog"
	"github.com/brocaar/loraserver/internal/gateway"
	"github.com/brocaar/loraserver/internal/gps"
	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/loraserver/internal/test"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/backend"
)

func TestNetworkServerAPI(t *testing.T) {
	conf := test.GetConfig()
	db, err := common.OpenDatabase(conf.PostgresDSN)
	if err != nil {
		t.Fatal(err)
	}
	config.C.PostgreSQL.DB = db
	config.C.Redis.Pool = common.NewRedisPool(conf.RedisURL)
	config.C.NetworkServer.NetID = [3]byte{1, 2, 3}

	gateway.MustSetStatsAggregationIntervals([]string{"MINUTE"})

	Convey("Given a clean PostgreSQL and Redis database + api instance", t, func() {
		test.MustResetDB(db)
		test.MustFlushRedis(config.C.Redis.Pool)

		grpcServer := grpc.NewServer()
		apiServer := NewNetworkServerAPI()
		ns.RegisterNetworkServerServer(grpcServer, apiServer)

		ln, err := net.Listen("tcp", "localhost:0")
		So(err, ShouldBeNil)
		go grpcServer.Serve(ln)
		defer func() {
			grpcServer.Stop()
			ln.Close()
		}()

		apiClient, err := grpc.Dial(ln.Addr().String(), grpc.WithInsecure(), grpc.WithBlock())
		So(err, ShouldBeNil)

		defer apiClient.Close()

		api := ns.NewNetworkServerClient(apiClient)
		ctx := context.Background()

		devEUI := [8]byte{1, 2, 3, 4, 5, 6, 7, 8}
		devAddr := [4]byte{6, 2, 3, 4}
		nwkSKey := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}

		Convey("When calling StreamFrameLogsForGateway", func() {
			mac := lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}
			respChan := make(chan *ns.StreamFrameLogsForGatewayResponse)

			client, err := api.StreamFrameLogsForGateway(ctx, &ns.StreamFrameLogsForGatewayRequest{
				Mac: mac[:],
			})
			So(err, ShouldBeNil)

			// some time for subscribing
			time.Sleep(100 * time.Millisecond)

			go func() {
				for {
					resp, err := client.Recv()
					if err != nil {
						break
					}
					respChan <- resp
				}
			}()

			Convey("When logging a downlink gateway frame", func() {
				dr0, err := config.C.NetworkServer.Band.Band.GetDataRate(0)
				So(err, ShouldBeNil)

				So(framelog.LogDownlinkFrameForGateway(framelog.DownlinkFrameLog{
					TXInfo: gw.TXInfo{
						MAC:      mac,
						DataRate: dr0,
					},
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataDown,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{},
					},
				}), ShouldBeNil)

				Convey("Then the frame-log was received by the client", func() {
					resp := <-respChan
					So(resp.UplinkFrames, ShouldHaveLength, 0)
					So(resp.DownlinkFrames, ShouldHaveLength, 1)
				})
			})

			Convey("When logging an uplink gateway frame", func() {
				So(framelog.LogUplinkFrameForGateways(models.RXPacket{
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{},
					},
					TXInfo: models.TXInfo{},
					RXInfoSet: []models.RXInfo{
						{
							MAC: mac,
						},
					},
				}), ShouldBeNil)

				Convey("Then the frame-log was received by the client", func() {
					resp := <-respChan
					So(resp.UplinkFrames, ShouldHaveLength, 1)
					So(resp.DownlinkFrames, ShouldHaveLength, 0)
				})
			})
		})

		Convey("When calling StreamFrameLogsForDevice", func() {
			respChan := make(chan *ns.StreamFrameLogsForDeviceResponse)

			client, err := api.StreamFrameLogsForDevice(ctx, &ns.StreamFrameLogsForDeviceRequest{
				DevEUI: devEUI[:],
			})
			So(err, ShouldBeNil)

			// some time for subscribing
			time.Sleep(100 * time.Millisecond)

			go func() {
				for {
					resp, err := client.Recv()
					if err != nil {
						break
					}
					respChan <- resp
				}
			}()

			Convey("When logging a downlink device frame", func() {
				dr0, err := config.C.NetworkServer.Band.Band.GetDataRate(0)
				So(err, ShouldBeNil)

				So(framelog.LogDownlinkFrameForDevEUI(devEUI, framelog.DownlinkFrameLog{
					TXInfo: gw.TXInfo{
						DataRate: dr0,
					},
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataDown,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{},
					},
				}), ShouldBeNil)

				Convey("Then the frame-log was received by the client", func() {
					resp := <-respChan
					So(resp.UplinkFrames, ShouldHaveLength, 0)
					So(resp.DownlinkFrames, ShouldHaveLength, 1)
				})
			})

			Convey("When logging an uplink device frame", func() {
				So(framelog.LogUplinkFrameForDevEUI(devEUI, models.RXPacket{
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{},
					},
					TXInfo: models.TXInfo{},
				}), ShouldBeNil)

				Convey("Then the frame-log was received by the client", func() {
					resp := <-respChan
					So(resp.UplinkFrames, ShouldHaveLength, 1)
					So(resp.DownlinkFrames, ShouldHaveLength, 0)
				})
			})
		})

		Convey("When calling CreateServiceProfile", func() {
			resp, err := api.CreateServiceProfile(ctx, &ns.CreateServiceProfileRequest{
				ServiceProfile: &ns.ServiceProfile{
					UlRate:                 1,
					UlBucketSize:           2,
					UlRatePolicy:           ns.RatePolicy_DROP,
					DlRate:                 3,
					DlBucketSize:           4,
					DlRatePolicy:           ns.RatePolicy_MARK,
					AddGWMetadata:          true,
					DevStatusReqFreq:       4,
					ReportDevStatusBattery: true,
					ReportDevStatusMargin:  true,
					DrMin:          5,
					DrMax:          6,
					ChannelMask:    []byte{1, 2, 3},
					PrAllowed:      true,
					HrAllowed:      true,
					RaAllowed:      true,
					NwkGeoLoc:      true,
					TargetPER:      1,
					MinGWDiversity: 7,
				},
			})
			So(err, ShouldBeNil)
			So(resp.ServiceProfileID, ShouldNotEqual, "")

			Convey("Then GetServiceProfile returns the service-profile", func() {
				getResp, err := api.GetServiceProfile(ctx, &ns.GetServiceProfileRequest{
					ServiceProfileID: resp.ServiceProfileID,
				})
				So(err, ShouldBeNil)
				So(getResp.ServiceProfile, ShouldResemble, &ns.ServiceProfile{
					ServiceProfileID:       resp.ServiceProfileID,
					UlRate:                 1,
					UlBucketSize:           2,
					UlRatePolicy:           ns.RatePolicy_DROP,
					DlRate:                 3,
					DlBucketSize:           4,
					DlRatePolicy:           ns.RatePolicy_MARK,
					AddGWMetadata:          true,
					DevStatusReqFreq:       4,
					ReportDevStatusBattery: true,
					ReportDevStatusMargin:  true,
					DrMin:          5,
					DrMax:          6,
					ChannelMask:    []byte{1, 2, 3},
					PrAllowed:      true,
					HrAllowed:      true,
					RaAllowed:      true,
					NwkGeoLoc:      true,
					TargetPER:      1,
					MinGWDiversity: 7,
				})
			})

			Convey("Then UpdateServiceProfile updates the service-profile", func() {
				_, err := api.UpdateServiceProfile(ctx, &ns.UpdateServiceProfileRequest{
					ServiceProfile: &ns.ServiceProfile{
						ServiceProfileID:       resp.ServiceProfileID,
						UlRate:                 2,
						UlBucketSize:           3,
						UlRatePolicy:           ns.RatePolicy_MARK,
						DlRate:                 4,
						DlBucketSize:           5,
						DlRatePolicy:           ns.RatePolicy_DROP,
						AddGWMetadata:          false,
						DevStatusReqFreq:       6,
						ReportDevStatusBattery: false,
						ReportDevStatusMargin:  false,
						DrMin:          7,
						DrMax:          8,
						ChannelMask:    []byte{3, 2, 1},
						PrAllowed:      false,
						HrAllowed:      false,
						RaAllowed:      false,
						NwkGeoLoc:      false,
						TargetPER:      2,
						MinGWDiversity: 8,
					},
				})
				So(err, ShouldBeNil)

				getResp, err := api.GetServiceProfile(ctx, &ns.GetServiceProfileRequest{
					ServiceProfileID: resp.ServiceProfileID,
				})
				So(err, ShouldBeNil)
				So(getResp.ServiceProfile, ShouldResemble, &ns.ServiceProfile{
					ServiceProfileID:       resp.ServiceProfileID,
					UlRate:                 2,
					UlBucketSize:           3,
					UlRatePolicy:           ns.RatePolicy_MARK,
					DlRate:                 4,
					DlBucketSize:           5,
					DlRatePolicy:           ns.RatePolicy_DROP,
					AddGWMetadata:          false,
					DevStatusReqFreq:       6,
					ReportDevStatusBattery: false,
					ReportDevStatusMargin:  false,
					DrMin:          7,
					DrMax:          8,
					ChannelMask:    []byte{3, 2, 1},
					PrAllowed:      false,
					HrAllowed:      false,
					RaAllowed:      false,
					NwkGeoLoc:      false,
					TargetPER:      2,
					MinGWDiversity: 8,
				})
			})

			Convey("Then DeleteServiceProfile deletes the service-profile", func() {
				_, err := api.DeleteServiceProfile(ctx, &ns.DeleteServiceProfileRequest{
					ServiceProfileID: resp.ServiceProfileID,
				})
				So(err, ShouldBeNil)

				_, err = api.DeleteServiceProfile(ctx, &ns.DeleteServiceProfileRequest{
					ServiceProfileID: resp.ServiceProfileID,
				})
				So(err, ShouldNotBeNil)
				So(grpc.Code(err), ShouldEqual, codes.NotFound)
			})
		})

		Convey("When calling CreateRoutingProfile", func() {
			resp, err := api.CreateRoutingProfile(ctx, &ns.CreateRoutingProfileRequest{
				RoutingProfile: &ns.RoutingProfile{
					AsID: "application-server:1234",
				},
				CaCert:  "CACERT",
				TlsCert: "TLSCERT",
				TlsKey:  "TLSKEY",
			})
			So(err, ShouldBeNil)
			So(resp.RoutingProfileID, ShouldNotEqual, "")

			Convey("Then GetRoutingProfile returns the routing-profile", func() {
				getResp, err := api.GetRoutingProfile(ctx, &ns.GetRoutingProfileRequest{
					RoutingProfileID: resp.RoutingProfileID,
				})
				So(err, ShouldBeNil)
				So(getResp.RoutingProfile, ShouldResemble, &ns.RoutingProfile{
					AsID: "application-server:1234",
				})
				So(getResp.CaCert, ShouldEqual, "CACERT")
				So(getResp.TlsCert, ShouldEqual, "TLSCERT")
			})

			Convey("Then UpdateRoutingProfile updates the routing-profile", func() {
				_, err := api.UpdateRoutingProfile(ctx, &ns.UpdateRoutingProfileRequest{
					RoutingProfile: &ns.RoutingProfile{
						RoutingProfileID: resp.RoutingProfileID,
						AsID:             "new-application-server:1234",
					},
					CaCert:  "CACERT2",
					TlsCert: "TLSCERT2",
					TlsKey:  "TLSKEY2",
				})
				So(err, ShouldBeNil)

				getResp, err := api.GetRoutingProfile(ctx, &ns.GetRoutingProfileRequest{
					RoutingProfileID: resp.RoutingProfileID,
				})
				So(err, ShouldBeNil)
				So(getResp.RoutingProfile, ShouldResemble, &ns.RoutingProfile{
					AsID: "new-application-server:1234",
				})
				So(getResp.CaCert, ShouldEqual, "CACERT2")
				So(getResp.TlsCert, ShouldEqual, "TLSCERT2")
			})

			Convey("Then DeleteRoutingProfile deletes the routing-profile", func() {
				_, err := api.DeleteRoutingProfile(ctx, &ns.DeleteRoutingProfileRequest{
					RoutingProfileID: resp.RoutingProfileID,
				})
				So(err, ShouldBeNil)

				_, err = api.DeleteRoutingProfile(ctx, &ns.DeleteRoutingProfileRequest{
					RoutingProfileID: resp.RoutingProfileID,
				})
				So(err, ShouldNotBeNil)
				So(grpc.Code(err), ShouldEqual, codes.NotFound)
			})
		})

		Convey("When calling CreateDeviceProfile", func() {
			resp, err := api.CreateDeviceProfile(ctx, &ns.CreateDeviceProfileRequest{
				DeviceProfile: &ns.DeviceProfile{
					SupportsClassB:     true,
					ClassBTimeout:      1,
					PingSlotPeriod:     2,
					PingSlotDR:         3,
					PingSlotFreq:       868100000,
					SupportsClassC:     true,
					ClassCTimeout:      4,
					MacVersion:         "1.0.2",
					RegParamsRevision:  "B",
					RxDelay1:           5,
					RxDROffset1:        6,
					RxDataRate2:        7,
					RxFreq2:            868200000,
					FactoryPresetFreqs: []uint32{868100000, 868300000, 868500000},
					MaxEIRP:            14,
					MaxDutyCycle:       1,
					SupportsJoin:       true,
					Supports32BitFCnt:  true,
				},
			})
			So(err, ShouldBeNil)
			So(resp.DeviceProfileID, ShouldNotEqual, "")

			Convey("Then GetDeviceProfile returns the device-profile", func() {
				getResp, err := api.GetDeviceProfile(ctx, &ns.GetDeviceProfileRequest{
					DeviceProfileID: resp.DeviceProfileID,
				})
				So(err, ShouldBeNil)
				So(getResp.DeviceProfile, ShouldResemble, &ns.DeviceProfile{
					SupportsClassB:     true,
					ClassBTimeout:      1,
					PingSlotPeriod:     2,
					PingSlotDR:         3,
					PingSlotFreq:       868100000,
					SupportsClassC:     true,
					ClassCTimeout:      4,
					MacVersion:         "1.0.2",
					RegParamsRevision:  "B",
					RxDelay1:           5,
					RxDROffset1:        6,
					RxDataRate2:        7,
					RxFreq2:            868200000,
					FactoryPresetFreqs: []uint32{868100000, 868300000, 868500000},
					MaxEIRP:            14,
					MaxDutyCycle:       1,
					SupportsJoin:       true,
					RfRegion:           "EU868", // set by the api
					Supports32BitFCnt:  true,
				})
			})
		})

		Convey("Given a ServiceProfile, RoutingProfile and DeviceProfile", func() {
			sp := storage.ServiceProfile{
				ServiceProfile: backend.ServiceProfile{},
			}
			So(storage.CreateServiceProfile(config.C.PostgreSQL.DB, &sp), ShouldBeNil)

			rp := storage.RoutingProfile{
				RoutingProfile: backend.RoutingProfile{},
			}
			So(storage.CreateRoutingProfile(config.C.PostgreSQL.DB, &rp), ShouldBeNil)

			dp := storage.DeviceProfile{
				DeviceProfile: backend.DeviceProfile{
					FactoryPresetFreqs: []backend.Frequency{
						868100000,
						868300000,
						868500000,
					},
				},
			}
			So(storage.CreateDeviceProfile(config.C.PostgreSQL.DB, &dp), ShouldBeNil)

			Convey("When calling CreateDevice", func() {
				_, err := api.CreateDevice(ctx, &ns.CreateDeviceRequest{
					Device: &ns.Device{
						DevEUI:           devEUI[:],
						DeviceProfileID:  dp.DeviceProfile.DeviceProfileID,
						ServiceProfileID: sp.ServiceProfile.ServiceProfileID,
						RoutingProfileID: rp.RoutingProfile.RoutingProfileID,
					},
				})
				So(err, ShouldBeNil)

				Convey("Then GetDevice returns the device", func() {
					resp, err := api.GetDevice(ctx, &ns.GetDeviceRequest{
						DevEUI: devEUI[:],
					})
					So(err, ShouldBeNil)
					So(resp.Device, ShouldResemble, &ns.Device{
						DevEUI:           devEUI[:],
						DeviceProfileID:  dp.DeviceProfile.DeviceProfileID,
						ServiceProfileID: sp.ServiceProfile.ServiceProfileID,
						RoutingProfileID: rp.RoutingProfile.RoutingProfileID,
					})
				})

				Convey("Then UpdateDevice updates the device", func() {
					rp2Resp, err := api.CreateRoutingProfile(ctx, &ns.CreateRoutingProfileRequest{
						RoutingProfile: &ns.RoutingProfile{
							AsID: "new-application-server:1234",
						},
					})
					So(err, ShouldBeNil)

					_, err = api.UpdateDevice(ctx, &ns.UpdateDeviceRequest{
						Device: &ns.Device{
							DevEUI:           devEUI[:],
							DeviceProfileID:  dp.DeviceProfile.DeviceProfileID,
							ServiceProfileID: sp.ServiceProfile.ServiceProfileID,
							RoutingProfileID: rp2Resp.RoutingProfileID,
						},
					})
					So(err, ShouldBeNil)

					resp, err := api.GetDevice(ctx, &ns.GetDeviceRequest{
						DevEUI: devEUI[:],
					})
					So(err, ShouldBeNil)
					So(resp.Device, ShouldResemble, &ns.Device{
						DevEUI:           devEUI[:],
						DeviceProfileID:  dp.DeviceProfile.DeviceProfileID,
						ServiceProfileID: sp.ServiceProfile.ServiceProfileID,
						RoutingProfileID: rp2Resp.RoutingProfileID,
					})
				})

				Convey("Then DeleteDevice deletes the device", func() {
					_, err := api.DeleteDevice(ctx, &ns.DeleteDeviceRequest{
						DevEUI: devEUI[:],
					})
					So(err, ShouldBeNil)

					_, err = api.DeleteDevice(ctx, &ns.DeleteDeviceRequest{
						DevEUI: devEUI[:],
					})
					So(err, ShouldNotBeNil)
					So(grpc.Code(err), ShouldEqual, codes.NotFound)
				})
			})
		})

		Convey("Given a ServiceProfile, RoutingProfile, DeviceProfile and Device", func() {
			sp := storage.ServiceProfile{
				ServiceProfile: backend.ServiceProfile{
					DRMin: 3,
					DRMax: 6,
				},
			}
			So(storage.CreateServiceProfile(config.C.PostgreSQL.DB, &sp), ShouldBeNil)

			rp := storage.RoutingProfile{
				RoutingProfile: backend.RoutingProfile{},
			}
			So(storage.CreateRoutingProfile(config.C.PostgreSQL.DB, &rp), ShouldBeNil)

			dp := storage.DeviceProfile{
				DeviceProfile: backend.DeviceProfile{
					FactoryPresetFreqs: []backend.Frequency{
						868100000,
						868300000,
						868500000,
					},
					RXDelay1:       3,
					RXDROffset1:    2,
					RXDataRate2:    5,
					RXFreq2:        868900000,
					PingSlotPeriod: 32,
					PingSlotFreq:   868100000,
					PingSlotDR:     5,
				},
			}
			So(storage.CreateDeviceProfile(config.C.PostgreSQL.DB, &dp), ShouldBeNil)

			d := storage.Device{
				DevEUI:           devEUI,
				DeviceProfileID:  dp.DeviceProfileID,
				RoutingProfileID: rp.RoutingProfileID,
				ServiceProfileID: sp.ServiceProfileID,
			}
			So(storage.CreateDevice(config.C.PostgreSQL.DB, &d), ShouldBeNil)

			Convey("Given an item in the device-queue", func() {
				_, err := api.CreateDeviceQueueItem(ctx, &ns.CreateDeviceQueueItemRequest{
					Item: &ns.DeviceQueueItem{
						DevEUI:     d.DevEUI[:],
						FrmPayload: []byte{1, 2, 3, 4},
						FCnt:       10,
						FPort:      20,
					},
				})
				So(err, ShouldBeNil)

				Convey("When calling ActivateDevice", func() {
					_, err := api.ActivateDevice(ctx, &ns.ActivateDeviceRequest{
						DevEUI:        devEUI[:],
						DevAddr:       devAddr[:],
						NwkSKey:       nwkSKey[:],
						FCntUp:        10,
						FCntDown:      11,
						SkipFCntCheck: true,
					})
					So(err, ShouldBeNil)

					Convey("Then the device-queue was flushed", func() {
						items, err := storage.GetDeviceQueueItemsForDevEUI(config.C.PostgreSQL.DB, d.DevEUI)
						So(err, ShouldBeNil)
						So(items, ShouldHaveLength, 0)
					})

					Convey("Then the device was activated as expected", func() {
						ds, err := storage.GetDeviceSession(config.C.Redis.Pool, devEUI)
						So(err, ShouldBeNil)
						So(ds, ShouldResemble, storage.DeviceSession{
							DeviceProfileID:  dp.DeviceProfile.DeviceProfileID,
							ServiceProfileID: sp.ServiceProfile.ServiceProfileID,
							RoutingProfileID: rp.RoutingProfile.RoutingProfileID,

							DevAddr:               devAddr,
							DevEUI:                devEUI,
							NwkSKey:               nwkSKey,
							FCntUp:                10,
							FCntDown:              11,
							SkipFCntValidation:    true,
							EnabledUplinkChannels: config.C.NetworkServer.Band.Band.GetEnabledUplinkChannelIndices(),
							ChannelFrequencies:    []int{868100000, 868300000, 868500000},
							ExtraUplinkChannels:   map[int]band.Channel{},
							RXDelay:               3,
							RX1DROffset:           2,
							RX2DR:                 5,
							RX2Frequency:          868900000,
							MaxSupportedDR:        6,

							LastDevStatusMargin: 127,
							PingSlotNb:          128,
							PingSlotDR:          5,
							PingSlotFrequency:   868100000,
						})
					})

					Convey("Then GetDeviceActivation returns the expected response", func() {
						resp, err := api.GetDeviceActivation(ctx, &ns.GetDeviceActivationRequest{
							DevEUI: devEUI[:],
						})
						So(err, ShouldBeNil)
						So(resp, ShouldResemble, &ns.GetDeviceActivationResponse{
							DevAddr:       devAddr[:],
							NwkSKey:       nwkSKey[:],
							FCntUp:        10,
							FCntDown:      11,
							SkipFCntCheck: true,
						})
					})

					Convey("Then GetNextDownlinkFCntForDevEUI returns the expected FCnt", func() {
						resp, err := api.GetNextDownlinkFCntForDevEUI(ctx, &ns.GetNextDownlinkFCntForDevEUIRequest{
							DevEUI: devEUI[:],
						})
						So(err, ShouldBeNil)
						So(resp.FCnt, ShouldEqual, 11)
					})

					Convey("Given an item in the device-queue", func() {
						_, err := api.CreateDeviceQueueItem(ctx, &ns.CreateDeviceQueueItemRequest{
							Item: &ns.DeviceQueueItem{
								DevEUI:     d.DevEUI[:],
								FrmPayload: []byte{1, 2, 3, 4},
								FCnt:       11,
								FPort:      20,
							},
						})
						So(err, ShouldBeNil)

						Convey("Then GetNextDownlinkFCntForDevEUI returns the expected FCnt", func() {
							resp, err := api.GetNextDownlinkFCntForDevEUI(ctx, &ns.GetNextDownlinkFCntForDevEUIRequest{
								DevEUI: devEUI[:],
							})
							So(err, ShouldBeNil)
							So(resp.FCnt, ShouldEqual, 12)
						})
					})

					Convey("Then DeactivateDevice deactivates the device and flushes the queue", func() {
						_, err := api.CreateDeviceQueueItem(ctx, &ns.CreateDeviceQueueItemRequest{
							Item: &ns.DeviceQueueItem{
								DevEUI:     d.DevEUI[:],
								FrmPayload: []byte{1, 2, 3, 4},
								FCnt:       10,
								FPort:      20,
							},
						})
						So(err, ShouldBeNil)

						items, err := storage.GetDeviceQueueItemsForDevEUI(config.C.PostgreSQL.DB, d.DevEUI)
						So(err, ShouldBeNil)
						So(items, ShouldHaveLength, 1)

						_, err = api.DeactivateDevice(ctx, &ns.DeactivateDeviceRequest{
							DevEUI: devEUI[:],
						})
						So(err, ShouldBeNil)

						_, err = api.GetDeviceActivation(ctx, &ns.GetDeviceActivationRequest{
							DevEUI: devEUI[:],
						})
						So(grpc.Code(err), ShouldEqual, codes.NotFound)

						items, err = storage.GetDeviceQueueItemsForDevEUI(config.C.PostgreSQL.DB, d.DevEUI)
						So(err, ShouldBeNil)
						So(items, ShouldHaveLength, 0)
					})

					Convey("When calling CreateMACCommandQueueItem", func() {
						mac := lorawan.MACCommand{
							CID: lorawan.RXParamSetupReq,
							Payload: &lorawan.RXParamSetupReqPayload{
								Frequency: 868100000,
							},
						}
						b, err := mac.MarshalBinary()
						So(err, ShouldBeNil)

						_, err = api.CreateMACCommandQueueItem(ctx, &ns.CreateMACCommandQueueItemRequest{
							DevEUI:   devEUI[:],
							Cid:      uint32(lorawan.RXParamSetupReq),
							Commands: [][]byte{b},
						})
						So(err, ShouldBeNil)

						Convey("Then the mac-command has been added to the queue", func() {
							queue, err := storage.GetMACCommandQueueItems(config.C.Redis.Pool, devEUI)
							So(err, ShouldBeNil)
							So(queue, ShouldResemble, []storage.MACCommandBlock{
								{
									CID:         lorawan.RXParamSetupReq,
									External:    true,
									MACCommands: []lorawan.MACCommand{mac},
								},
							})
						})
					})
				})
			})

			Convey("Given the device is in Class-B mode", func() {
				dp.SupportsClassB = true
				dp.ClassBTimeout = 30
				So(storage.UpdateDeviceProfile(config.C.PostgreSQL.DB, &dp), ShouldBeNil)

				ds := storage.DeviceSession{
					DevEUI:       d.DevEUI,
					BeaconLocked: true,
					PingSlotNb:   1,
				}
				So(storage.SaveDeviceSession(config.C.Redis.Pool, ds), ShouldBeNil)

				Convey("When calling CreateDeviceQueueItem", func() {
					_, err := api.CreateDeviceQueueItem(ctx, &ns.CreateDeviceQueueItemRequest{
						Item: &ns.DeviceQueueItem{
							DevEUI:     d.DevEUI[:],
							FrmPayload: []byte{1, 2, 3, 4},
							FCnt:       10,
							FPort:      20,
							Confirmed:  true,
						},
					})
					So(err, ShouldBeNil)

					Convey("Then the GPS epoch timestamp and timeout are set", func() {
						queueItems, err := storage.GetDeviceQueueItemsForDevEUI(config.C.PostgreSQL.DB, d.DevEUI)
						So(err, ShouldBeNil)
						So(queueItems, ShouldHaveLength, 1)

						So(queueItems[0].EmitAtTimeSinceGPSEpoch, ShouldNotBeNil)
						So(queueItems[0].TimeoutAfter, ShouldNotBeNil)

						emitAt := time.Time(gps.NewFromTimeSinceGPSEpoch(*queueItems[0].EmitAtTimeSinceGPSEpoch))
						So(emitAt.After(time.Now()), ShouldBeTrue)

						So(queueItems[0].TimeoutAfter.Equal(emitAt.Add(time.Second*time.Duration(dp.ClassBTimeout))), ShouldBeTrue)
					})

					Convey("When calling enqueueing a second item", func() {
						_, err := api.CreateDeviceQueueItem(ctx, &ns.CreateDeviceQueueItemRequest{
							Item: &ns.DeviceQueueItem{
								DevEUI:     d.DevEUI[:],
								FrmPayload: []byte{1, 2, 3, 4},
								FCnt:       11,
								FPort:      20,
								Confirmed:  true,
							},
						})
						So(err, ShouldBeNil)

						Convey("Then the GPS timestamp occurs after the first queue item", func() {
							queueItems, err := storage.GetDeviceQueueItemsForDevEUI(config.C.PostgreSQL.DB, d.DevEUI)
							So(err, ShouldBeNil)
							So(queueItems, ShouldHaveLength, 2)
							So(queueItems[0].EmitAtTimeSinceGPSEpoch, ShouldNotBeNil)
							So(queueItems[1].EmitAtTimeSinceGPSEpoch, ShouldNotBeNil)
							So(*queueItems[1].EmitAtTimeSinceGPSEpoch, ShouldBeGreaterThan, *queueItems[0].EmitAtTimeSinceGPSEpoch)
						})
					})
				})
			})

			Convey("When calling CreateDeviceQueueItem", func() {
				_, err := api.CreateDeviceQueueItem(ctx, &ns.CreateDeviceQueueItemRequest{
					Item: &ns.DeviceQueueItem{
						DevEUI:     d.DevEUI[:],
						FrmPayload: []byte{1, 2, 3, 4},
						FCnt:       10,
						FPort:      20,
						Confirmed:  true,
					},
				})
				So(err, ShouldBeNil)

				Convey("Then GetDeviceQueueItemsForDevEUI returns the item", func() {
					resp, err := api.GetDeviceQueueItemsForDevEUI(ctx, &ns.GetDeviceQueueItemsForDevEUIRequest{
						DevEUI: d.DevEUI[:],
					})
					So(err, ShouldBeNil)
					So(resp.Items, ShouldHaveLength, 1)
					So(resp.Items[0], ShouldResemble, &ns.DeviceQueueItem{
						DevEUI:     d.DevEUI[:],
						FrmPayload: []byte{1, 2, 3, 4},
						FCnt:       10,
						FPort:      20,
						Confirmed:  true,
					})
				})

				Convey("Then FlushDeviceQueueForDevEUI flushes the device-queue", func() {
					_, err := api.FlushDeviceQueueForDevEUI(ctx, &ns.FlushDeviceQueueForDevEUIRequest{
						DevEUI: d.DevEUI[:],
					})
					So(err, ShouldBeNil)

					resp, err := api.GetDeviceQueueItemsForDevEUI(ctx, &ns.GetDeviceQueueItemsForDevEUIRequest{
						DevEUI: d.DevEUI[:],
					})
					So(err, ShouldBeNil)
					So(resp.Items, ShouldHaveLength, 0)
				})
			})

			Convey("When calling GetRandomDevAddr", func() {
				resp, err := api.GetRandomDevAddr(ctx, &ns.GetRandomDevAddrRequest{})
				So(err, ShouldBeNil)

				Convey("A random DevAddr has been returned", func() {
					So(resp.DevAddr, ShouldHaveLength, 4)
					So(resp.DevAddr, ShouldNotResemble, []byte{0, 0, 0, 0})
				})
			})
		})

		Convey("When calling CreateGateway", func() {
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

			Convey("Then the gateway has been created", func() {
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

			Convey("Then UpdateGateway updates the gateway", func() {
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

			Convey("Then ListGateways returns the gateway", func() {
				resp, err := api.ListGateways(ctx, &ns.ListGatewayRequest{
					Limit:  10,
					Offset: 0,
				})
				So(err, ShouldBeNil)
				So(resp.TotalCount, ShouldEqual, 1)
				So(resp.Result, ShouldHaveLength, 1)
				So(resp.Result[0].Mac, ShouldResemble, []byte{1, 2, 3, 4, 5, 6, 7, 8})
			})

			Convey("Then DeleteGateway deletes the gateway", func() {
				_, err := api.DeleteGateway(ctx, &ns.DeleteGatewayRequest{
					Mac: []byte{1, 2, 3, 4, 5, 6, 7, 8},
				})
				So(err, ShouldBeNil)

				_, err = api.GetGateway(ctx, &ns.GetGatewayRequest{
					Mac: []byte{1, 2, 3, 4, 5, 6, 7, 8},
				})
				So(err, ShouldResemble, grpc.Errorf(codes.NotFound, "gateway does not exist"))
			})

			Convey("When calling GenerateGatewayToken", func() {
				config.C.NetworkServer.Gateway.API.JWTSecret = "verysecret"

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

				Convey("Then GetGatewayStats returns these stats", func() {
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

			Convey("When calling CreateChannelConfiguration", func() {
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

					Convey("Then ListChannelConfigurations returns the channel-configuration", func() {
						cfs, err := api.ListChannelConfigurations(ctx, &ns.ListChannelConfigurationsRequest{})
						So(err, ShouldBeNil)
						So(cfs.Result, ShouldHaveLength, 1)
						So(cfs.Result[0], ShouldResemble, cf)
					})

					Convey("Then UpdateChannelConfiguration updates the channel-configuration", func() {
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

				Convey("Then DeleteChannelConfiguration deletes the channel-configuration", func() {
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

				Convey("Then CreateExtraChannel creates an extra channel-configuration channel", func() {
					ecRes, err := api.CreateExtraChannel(ctx, &ns.CreateExtraChannelRequest{
						ChannelConfigurationID: cfResp.Id,
						Modulation:             ns.Modulation_LORA,
						Frequency:              867100000,
						BandWidth:              125,
						SpreadFactors:          []int32{0, 1, 2, 3, 4, 5},
					})
					So(err, ShouldBeNil)
					So(ecRes.Id, ShouldNotEqual, 0)

					Convey("Then UpdateExtraChannel updates this extra channel", func() {
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

					Convey("Then DeleteExtraChannel deletes this extra channel", func() {
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

			Convey("Then GetVersion returns the expected value", func() {
				config.Version = "1.2.3"

				resp, err := api.GetVersion(ctx, &ns.GetVersionRequest{})
				So(err, ShouldBeNil)
				So(resp, ShouldResemble, &ns.GetVersionResponse{
					Version: "1.2.3",
					Region:  ns.Region_EU868,
				})
			})
		})
	})
}
