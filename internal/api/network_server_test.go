package api

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/gofrs/uuid"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/empty"
	. "github.com/smartystreets/goconvey/convey"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	commonPB "github.com/brocaar/loraserver/api/common"
	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/api/ns"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/framelog"
	"github.com/brocaar/loraserver/internal/gps"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/loraserver/internal/test"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/band"
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

	storage.MustSetStatsAggregationIntervals([]string{"MINUTE"})

	Convey("Given a clean PostgreSQL and Redis database + api instance", t, func() {
		test.MustResetDB(db)
		test.MustFlushRedis(config.C.Redis.Pool)

		grpcServer := grpc.NewServer()
		apiServer := NewNetworkServerAPI()
		ns.RegisterNetworkServerServiceServer(grpcServer, apiServer)

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

		api := ns.NewNetworkServerServiceClient(apiClient)
		ctx := context.Background()

		devEUI := [8]byte{1, 2, 3, 4, 5, 6, 7, 8}
		devAddr := [4]byte{6, 2, 3, 4}
		sNwkSIntKey := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
		fNwkSIntKey := [16]byte{2, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
		nwkSEncKey := [16]byte{3, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}

		Convey("When calling StreamFrameLogsForGateway", func() {
			mac := lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}
			respChan := make(chan *ns.StreamFrameLogsForGatewayResponse)

			client, err := api.StreamFrameLogsForGateway(ctx, &ns.StreamFrameLogsForGatewayRequest{
				GatewayId: mac[:],
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
				So(framelog.LogDownlinkFrameForGateway(gw.DownlinkFrame{
					TxInfo: &gw.DownlinkTXInfo{
						GatewayId: mac[:],
					},
				}), ShouldBeNil)

				Convey("Then the frame-log was received by the client", func() {
					resp := <-respChan
					So(resp.GetDownlinkFrame(), ShouldNotBeNil)
					So(resp.GetUplinkFrameSet(), ShouldBeNil)
				})
			})

			Convey("When logging an uplink gateway frame", func() {
				So(framelog.LogUplinkFrameForGateways(gw.UplinkFrameSet{
					RxInfo: []*gw.UplinkRXInfo{
						{
							GatewayId: mac[:],
						},
					},
				}), ShouldBeNil)

				Convey("Then the frame-log was received by the client", func() {
					resp := <-respChan
					So(resp.GetDownlinkFrame(), ShouldBeNil)
					So(resp.GetUplinkFrameSet(), ShouldNotBeNil)
				})
			})
		})

		Convey("When calling StreamFrameLogsForDevice", func() {
			respChan := make(chan *ns.StreamFrameLogsForDeviceResponse)

			client, err := api.StreamFrameLogsForDevice(ctx, &ns.StreamFrameLogsForDeviceRequest{
				DevEui: devEUI[:],
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
				So(framelog.LogDownlinkFrameForDevEUI(devEUI, gw.DownlinkFrame{}), ShouldBeNil)

				Convey("Then the frame-log was received by the client", func() {
					resp := <-respChan
					So(resp.GetDownlinkFrame(), ShouldNotBeNil)
					So(resp.GetUplinkFrameSet(), ShouldBeNil)
				})
			})

			Convey("When logging an uplink device frame", func() {
				So(framelog.LogUplinkFrameForDevEUI(devEUI, gw.UplinkFrameSet{}), ShouldBeNil)

				Convey("Then the frame-log was received by the client", func() {
					resp := <-respChan
					So(resp.GetDownlinkFrame(), ShouldBeNil)
					So(resp.GetUplinkFrameSet(), ShouldNotBeNil)
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
					AddGwMetadata:          true,
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
					TargetPer:      1,
					MinGwDiversity: 7,
				},
			})
			So(err, ShouldBeNil)
			So(resp.Id, ShouldHaveLength, 16)
			So(resp.Id, ShouldNotResemble, uuid.Nil[:])

			Convey("Then GetServiceProfile returns the service-profile", func() {
				getResp, err := api.GetServiceProfile(ctx, &ns.GetServiceProfileRequest{
					Id: resp.Id,
				})
				So(err, ShouldBeNil)
				So(getResp.ServiceProfile, ShouldResemble, &ns.ServiceProfile{
					Id:                     resp.Id,
					UlRate:                 1,
					UlBucketSize:           2,
					UlRatePolicy:           ns.RatePolicy_DROP,
					DlRate:                 3,
					DlBucketSize:           4,
					DlRatePolicy:           ns.RatePolicy_MARK,
					AddGwMetadata:          true,
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
					TargetPer:      1,
					MinGwDiversity: 7,
				})
			})

			Convey("Then UpdateServiceProfile updates the service-profile", func() {
				_, err := api.UpdateServiceProfile(ctx, &ns.UpdateServiceProfileRequest{
					ServiceProfile: &ns.ServiceProfile{
						Id:                     resp.Id,
						UlRate:                 2,
						UlBucketSize:           3,
						UlRatePolicy:           ns.RatePolicy_MARK,
						DlRate:                 4,
						DlBucketSize:           5,
						DlRatePolicy:           ns.RatePolicy_DROP,
						AddGwMetadata:          false,
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
						TargetPer:      2,
						MinGwDiversity: 8,
					},
				})
				So(err, ShouldBeNil)

				getResp, err := api.GetServiceProfile(ctx, &ns.GetServiceProfileRequest{
					Id: resp.Id,
				})
				So(err, ShouldBeNil)
				So(getResp.ServiceProfile, ShouldResemble, &ns.ServiceProfile{
					Id:                     resp.Id,
					UlRate:                 2,
					UlBucketSize:           3,
					UlRatePolicy:           ns.RatePolicy_MARK,
					DlRate:                 4,
					DlBucketSize:           5,
					DlRatePolicy:           ns.RatePolicy_DROP,
					AddGwMetadata:          false,
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
					TargetPer:      2,
					MinGwDiversity: 8,
				})
			})

			Convey("Then DeleteServiceProfile deletes the service-profile", func() {
				_, err := api.DeleteServiceProfile(ctx, &ns.DeleteServiceProfileRequest{
					Id: resp.Id,
				})
				So(err, ShouldBeNil)

				_, err = api.DeleteServiceProfile(ctx, &ns.DeleteServiceProfileRequest{
					Id: resp.Id,
				})
				So(err, ShouldNotBeNil)
				So(grpc.Code(err), ShouldEqual, codes.NotFound)
			})
		})

		Convey("When calling CreateRoutingProfile", func() {
			resp, err := api.CreateRoutingProfile(ctx, &ns.CreateRoutingProfileRequest{
				RoutingProfile: &ns.RoutingProfile{
					AsId:    "application-server:1234",
					CaCert:  "CACERT",
					TlsCert: "TLSCERT",
					TlsKey:  "TLSKEY",
				},
			})
			So(err, ShouldBeNil)
			So(resp.Id, ShouldHaveLength, 16)
			So(resp.Id, ShouldNotResemble, uuid.Nil[:])

			Convey("Then GetRoutingProfile returns the routing-profile", func() {
				getResp, err := api.GetRoutingProfile(ctx, &ns.GetRoutingProfileRequest{
					Id: resp.Id,
				})
				So(err, ShouldBeNil)
				So(getResp.RoutingProfile, ShouldResemble, &ns.RoutingProfile{
					Id:      resp.Id,
					AsId:    "application-server:1234",
					CaCert:  "CACERT",
					TlsCert: "TLSCERT",
				})
			})

			Convey("Then UpdateRoutingProfile updates the routing-profile", func() {
				_, err := api.UpdateRoutingProfile(ctx, &ns.UpdateRoutingProfileRequest{
					RoutingProfile: &ns.RoutingProfile{
						Id:      resp.Id,
						AsId:    "new-application-server:1234",
						CaCert:  "CACERT2",
						TlsCert: "TLSCERT2",
						TlsKey:  "TLSKEY2",
					},
				})
				So(err, ShouldBeNil)

				getResp, err := api.GetRoutingProfile(ctx, &ns.GetRoutingProfileRequest{
					Id: resp.Id,
				})
				So(err, ShouldBeNil)
				So(getResp.RoutingProfile, ShouldResemble, &ns.RoutingProfile{
					Id:      resp.Id,
					AsId:    "new-application-server:1234",
					CaCert:  "CACERT2",
					TlsCert: "TLSCERT2",
				})
			})

			Convey("Then DeleteRoutingProfile deletes the routing-profile", func() {
				_, err := api.DeleteRoutingProfile(ctx, &ns.DeleteRoutingProfileRequest{
					Id: resp.Id,
				})
				So(err, ShouldBeNil)

				_, err = api.DeleteRoutingProfile(ctx, &ns.DeleteRoutingProfileRequest{
					Id: resp.Id,
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
					PingSlotDr:         3,
					PingSlotFreq:       868100000,
					SupportsClassC:     true,
					ClassCTimeout:      4,
					MacVersion:         "1.0.2",
					RegParamsRevision:  "B",
					RxDelay_1:          5,
					RxDrOffset_1:       6,
					RxDatarate_2:       7,
					RxFreq_2:           868200000,
					FactoryPresetFreqs: []uint32{868100000, 868300000, 868500000},
					MaxEirp:            14,
					MaxDutyCycle:       1,
					SupportsJoin:       true,
					Supports_32BitFCnt: true,
				},
			})
			So(err, ShouldBeNil)
			So(resp.Id, ShouldHaveLength, 16)
			So(resp.Id, ShouldNotEqual, uuid.Nil[:])

			Convey("Then GetDeviceProfile returns the device-profile", func() {
				getResp, err := api.GetDeviceProfile(ctx, &ns.GetDeviceProfileRequest{
					Id: resp.Id,
				})
				So(err, ShouldBeNil)
				So(getResp.DeviceProfile, ShouldResemble, &ns.DeviceProfile{
					Id:                 resp.Id,
					SupportsClassB:     true,
					ClassBTimeout:      1,
					PingSlotPeriod:     2,
					PingSlotDr:         3,
					PingSlotFreq:       868100000,
					SupportsClassC:     true,
					ClassCTimeout:      4,
					MacVersion:         "1.0.2",
					RegParamsRevision:  "B",
					RxDelay_1:          5,
					RxDrOffset_1:       6,
					RxDatarate_2:       7,
					RxFreq_2:           868200000,
					FactoryPresetFreqs: []uint32{868100000, 868300000, 868500000},
					MaxEirp:            14,
					MaxDutyCycle:       1,
					SupportsJoin:       true,
					RfRegion:           "EU868", // set by the api
					Supports_32BitFCnt: true,
				})
			})
		})

		Convey("Given a ServiceProfile, RoutingProfile and DeviceProfile", func() {
			sp := storage.ServiceProfile{}
			So(storage.CreateServiceProfile(config.C.PostgreSQL.DB, &sp), ShouldBeNil)

			rp := storage.RoutingProfile{}
			So(storage.CreateRoutingProfile(config.C.PostgreSQL.DB, &rp), ShouldBeNil)

			dp := storage.DeviceProfile{
				FactoryPresetFreqs: []int{
					868100000,
					868300000,
					868500000,
				},
			}
			So(storage.CreateDeviceProfile(config.C.PostgreSQL.DB, &dp), ShouldBeNil)

			Convey("When calling CreateDevice", func() {
				_, err := api.CreateDevice(ctx, &ns.CreateDeviceRequest{
					Device: &ns.Device{
						DevEui:           devEUI[:],
						DeviceProfileId:  dp.ID.Bytes(),
						ServiceProfileId: sp.ID.Bytes(),
						RoutingProfileId: rp.ID.Bytes(),
						SkipFCntCheck:    true,
					},
				})
				So(err, ShouldBeNil)

				Convey("Then GetDevice returns the device", func() {
					resp, err := api.GetDevice(ctx, &ns.GetDeviceRequest{
						DevEui: devEUI[:],
					})
					So(err, ShouldBeNil)
					So(resp.Device, ShouldResemble, &ns.Device{
						DevEui:           devEUI[:],
						DeviceProfileId:  dp.ID.Bytes(),
						ServiceProfileId: sp.ID.Bytes(),
						RoutingProfileId: rp.ID.Bytes(),
						SkipFCntCheck:    true,
					})
				})

				Convey("Then UpdateDevice updates the device", func() {
					rp2Resp, err := api.CreateRoutingProfile(ctx, &ns.CreateRoutingProfileRequest{
						RoutingProfile: &ns.RoutingProfile{
							AsId: "new-application-server:1234",
						},
					})
					So(err, ShouldBeNil)

					_, err = api.UpdateDevice(ctx, &ns.UpdateDeviceRequest{
						Device: &ns.Device{
							DevEui:           devEUI[:],
							DeviceProfileId:  dp.ID.Bytes(),
							ServiceProfileId: sp.ID.Bytes(),
							RoutingProfileId: rp2Resp.Id,
							SkipFCntCheck:    true,
						},
					})
					So(err, ShouldBeNil)

					resp, err := api.GetDevice(ctx, &ns.GetDeviceRequest{
						DevEui: devEUI[:],
					})
					So(err, ShouldBeNil)
					So(resp.Device, ShouldResemble, &ns.Device{
						DevEui:           devEUI[:],
						DeviceProfileId:  dp.ID.Bytes(),
						ServiceProfileId: sp.ID.Bytes(),
						RoutingProfileId: rp2Resp.Id,
						SkipFCntCheck:    true,
					})
				})

				Convey("Then DeleteDevice deletes the device", func() {
					_, err := api.DeleteDevice(ctx, &ns.DeleteDeviceRequest{
						DevEui: devEUI[:],
					})
					So(err, ShouldBeNil)

					_, err = api.DeleteDevice(ctx, &ns.DeleteDeviceRequest{
						DevEui: devEUI[:],
					})
					So(err, ShouldNotBeNil)
					So(grpc.Code(err), ShouldEqual, codes.NotFound)
				})
			})
		})

		Convey("Given a ServiceProfile, RoutingProfile, DeviceProfile and Device", func() {
			sp := storage.ServiceProfile{
				DRMin: 3,
				DRMax: 6,
			}
			So(storage.CreateServiceProfile(config.C.PostgreSQL.DB, &sp), ShouldBeNil)

			rp := storage.RoutingProfile{}
			So(storage.CreateRoutingProfile(config.C.PostgreSQL.DB, &rp), ShouldBeNil)

			dp := storage.DeviceProfile{
				FactoryPresetFreqs: []int{
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
				MACVersion:     "1.0.2",
			}
			So(storage.CreateDeviceProfile(config.C.PostgreSQL.DB, &dp), ShouldBeNil)

			d := storage.Device{
				DevEUI:           devEUI,
				DeviceProfileID:  dp.ID,
				RoutingProfileID: rp.ID,
				ServiceProfileID: sp.ID,
			}
			So(storage.CreateDevice(config.C.PostgreSQL.DB, &d), ShouldBeNil)

			Convey("Given an item in the device-queue", func() {
				_, err := api.CreateDeviceQueueItem(ctx, &ns.CreateDeviceQueueItemRequest{
					Item: &ns.DeviceQueueItem{
						DevEui:     devEUI[:],
						FrmPayload: []byte{1, 2, 3, 4},
						FCnt:       10,
						FPort:      20,
					},
				})
				So(err, ShouldBeNil)

				Convey("When calling ActivateDevice when the Device has SkipFCntCheck set to true", func() {
					d.SkipFCntCheck = true
					So(storage.UpdateDevice(db, &d), ShouldBeNil)

					_, err := api.ActivateDevice(ctx, &ns.ActivateDeviceRequest{
						DeviceActivation: &ns.DeviceActivation{
							DevEui:        devEUI[:],
							DevAddr:       devAddr[:],
							SNwkSIntKey:   sNwkSIntKey[:],
							FNwkSIntKey:   fNwkSIntKey[:],
							NwkSEncKey:    nwkSEncKey[:],
							FCntUp:        10,
							NFCntDown:     11,
							AFCntDown:     12,
							SkipFCntCheck: false,
						},
					})
					So(err, ShouldBeNil)

					Convey("Then SkipFCntCheck has been enabled in the activation", func() {
						ds, err := storage.GetDeviceSession(config.C.Redis.Pool, devEUI)
						So(err, ShouldBeNil)
						So(ds.SkipFCntValidation, ShouldBeTrue)
					})
				})

				Convey("When calling ActivateDevice", func() {
					_, err := api.ActivateDevice(ctx, &ns.ActivateDeviceRequest{
						DeviceActivation: &ns.DeviceActivation{
							DevEui:        devEUI[:],
							DevAddr:       devAddr[:],
							SNwkSIntKey:   sNwkSIntKey[:],
							FNwkSIntKey:   fNwkSIntKey[:],
							NwkSEncKey:    nwkSEncKey[:],
							FCntUp:        10,
							NFCntDown:     11,
							AFCntDown:     12,
							SkipFCntCheck: true,
						},
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
							DeviceProfileID:  dp.ID,
							ServiceProfileID: sp.ID,
							RoutingProfileID: rp.ID,

							DevAddr:               devAddr,
							DevEUI:                devEUI,
							SNwkSIntKey:           sNwkSIntKey,
							FNwkSIntKey:           fNwkSIntKey,
							NwkSEncKey:            nwkSEncKey,
							FCntUp:                10,
							NFCntDown:             11,
							AFCntDown:             12,
							SkipFCntValidation:    true,
							EnabledUplinkChannels: config.C.NetworkServer.Band.Band.GetEnabledUplinkChannelIndices(),
							ChannelFrequencies:    []int{868100000, 868300000, 868500000},
							ExtraUplinkChannels:   map[int]band.Channel{},
							RXDelay:               3,
							RX1DROffset:           2,
							RX2DR:                 5,
							RX2Frequency:          868900000,
							MaxSupportedDR:        6,
							UplinkGatewayHistory:  make(map[lorawan.EUI64]storage.UplinkGatewayHistory),
							PingSlotNb:            128,
							PingSlotDR:            5,
							PingSlotFrequency:     868100000,
							NbTrans:               1,
							MACVersion:            "1.0.2",
						})
					})

					Convey("Then GetDeviceActivation returns the expected response", func() {
						resp, err := api.GetDeviceActivation(ctx, &ns.GetDeviceActivationRequest{
							DevEui: devEUI[:],
						})
						So(err, ShouldBeNil)
						So(resp, ShouldResemble, &ns.GetDeviceActivationResponse{
							DeviceActivation: &ns.DeviceActivation{
								DevEui:        devEUI[:],
								DevAddr:       devAddr[:],
								SNwkSIntKey:   sNwkSIntKey[:],
								FNwkSIntKey:   fNwkSIntKey[:],
								NwkSEncKey:    nwkSEncKey[:],
								FCntUp:        10,
								NFCntDown:     11,
								AFCntDown:     12,
								SkipFCntCheck: true,
							},
						})
					})

					Convey("For LoRaWAN 1.0", func() {
						Convey("Then GetNextDownlinkFCntForDevEUI returns the expected FCnt", func() {
							resp, err := api.GetNextDownlinkFCntForDevEUI(ctx, &ns.GetNextDownlinkFCntForDevEUIRequest{
								DevEui: devEUI[:],
							})
							So(err, ShouldBeNil)
							So(resp.FCnt, ShouldEqual, 11)
						})
					})

					Convey("For LoRaWAN 1.1", func() {
						Convey("Then GetNextDownlinkFCntForDevEUI returns the expected FCnt", func() {
							ds, err := storage.GetDeviceSession(config.C.Redis.Pool, devEUI)
							So(err, ShouldBeNil)

							ds.MACVersion = "1.1.0"
							So(storage.SaveDeviceSession(config.C.Redis.Pool, ds), ShouldBeNil)

							resp, err := api.GetNextDownlinkFCntForDevEUI(ctx, &ns.GetNextDownlinkFCntForDevEUIRequest{
								DevEui: devEUI[:],
							})
							So(err, ShouldBeNil)
							So(resp.FCnt, ShouldEqual, 12)
						})
					})

					Convey("Given an item in the device-queue", func() {
						_, err := api.CreateDeviceQueueItem(ctx, &ns.CreateDeviceQueueItemRequest{
							Item: &ns.DeviceQueueItem{
								DevEui:     devEUI[:],
								FrmPayload: []byte{1, 2, 3, 4},
								FCnt:       11,
								FPort:      20,
							},
						})
						So(err, ShouldBeNil)

						Convey("Then GetNextDownlinkFCntForDevEUI returns the expected FCnt", func() {
							resp, err := api.GetNextDownlinkFCntForDevEUI(ctx, &ns.GetNextDownlinkFCntForDevEUIRequest{
								DevEui: devEUI[:],
							})
							So(err, ShouldBeNil)
							So(resp.FCnt, ShouldEqual, 12)
						})
					})

					Convey("Then DeactivateDevice deactivates the device and flushes the queue", func() {
						_, err := api.CreateDeviceQueueItem(ctx, &ns.CreateDeviceQueueItemRequest{
							Item: &ns.DeviceQueueItem{
								DevEui:     devEUI[:],
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
							DevEui: devEUI[:],
						})
						So(err, ShouldBeNil)

						_, err = api.GetDeviceActivation(ctx, &ns.GetDeviceActivationRequest{
							DevEui: devEUI[:],
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
							DevEui:   devEUI[:],
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
							DevEui:     devEUI[:],
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
								DevEui:     devEUI[:],
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
						DevEui:     devEUI[:],
						FrmPayload: []byte{1, 2, 3, 4},
						FCnt:       10,
						FPort:      20,
						Confirmed:  true,
					},
				})
				So(err, ShouldBeNil)

				Convey("Then GetDeviceQueueItemsForDevEUI returns the item", func() {
					resp, err := api.GetDeviceQueueItemsForDevEUI(ctx, &ns.GetDeviceQueueItemsForDevEUIRequest{
						DevEui: devEUI[:],
					})
					So(err, ShouldBeNil)
					So(resp.Items, ShouldHaveLength, 1)
					So(resp.Items[0], ShouldResemble, &ns.DeviceQueueItem{
						DevEui:     devEUI[:],
						FrmPayload: []byte{1, 2, 3, 4},
						FCnt:       10,
						FPort:      20,
						Confirmed:  true,
					})
				})

				Convey("Then FlushDeviceQueueForDevEUI flushes the device-queue", func() {
					_, err := api.FlushDeviceQueueForDevEUI(ctx, &ns.FlushDeviceQueueForDevEUIRequest{
						DevEui: devEUI[:],
					})
					So(err, ShouldBeNil)

					resp, err := api.GetDeviceQueueItemsForDevEUI(ctx, &ns.GetDeviceQueueItemsForDevEUIRequest{
						DevEui: devEUI[:],
					})
					So(err, ShouldBeNil)
					So(resp.Items, ShouldHaveLength, 0)
				})
			})

			Convey("When calling GetRandomDevAddr", func() {
				resp, err := api.GetRandomDevAddr(ctx, &empty.Empty{})
				So(err, ShouldBeNil)

				Convey("A random DevAddr has been returned", func() {
					So(resp.DevAddr, ShouldHaveLength, 4)
					So(resp.DevAddr, ShouldNotResemble, []byte{0, 0, 0, 0})
				})
			})
		})

		Convey("When calling CreateGateway", func() {
			req := ns.CreateGatewayRequest{
				Gateway: &ns.Gateway{
					Id: []byte{1, 2, 3, 4, 5, 6, 7, 8},
					Location: &commonPB.Location{
						Latitude:  1.1234,
						Longitude: 1.1235,
						Altitude:  15.5,
					},
				},
			}

			_, err := api.CreateGateway(ctx, &req)
			So(err, ShouldBeNil)
			req.Gateway.Location.XXX_sizecache = 0

			Convey("Then the gateway has been created", func() {
				resp, err := api.GetGateway(ctx, &ns.GetGatewayRequest{Id: req.Gateway.Id})
				So(err, ShouldBeNil)
				req.Gateway.XXX_sizecache = 0
				So(resp.Gateway, ShouldResemble, req.Gateway)
				So(resp.CreatedAt.String(), ShouldNotEqual, "")
				So(resp.UpdatedAt.String(), ShouldNotEqual, "")
				So(resp.FirstSeenAt, ShouldBeNil)
				So(resp.LastSeenAt, ShouldBeNil)
			})

			Convey("Then UpdateGateway updates the gateway", func() {
				req := ns.UpdateGatewayRequest{
					Gateway: &ns.Gateway{
						Id: []byte{1, 2, 3, 4, 5, 6, 7, 8},
						Location: &commonPB.Location{
							Latitude:  1.1235,
							Longitude: 1.1236,
							Altitude:  15.7,
						},
					},
				}
				_, err := api.UpdateGateway(ctx, &req)
				So(err, ShouldBeNil)
				req.Gateway.Location.XXX_sizecache = 0

				resp, err := api.GetGateway(ctx, &ns.GetGatewayRequest{Id: req.Gateway.Id})
				So(err, ShouldBeNil)
				req.Gateway.XXX_sizecache = 0
				So(resp.Gateway, ShouldResemble, req.Gateway)
				So(resp.CreatedAt.String(), ShouldNotEqual, "")
				So(resp.UpdatedAt.String(), ShouldNotEqual, "")
				So(resp.FirstSeenAt, ShouldBeNil)
				So(resp.LastSeenAt, ShouldBeNil)
			})

			Convey("Then DeleteGateway deletes the gateway", func() {
				_, err := api.DeleteGateway(ctx, &ns.DeleteGatewayRequest{
					Id: []byte{1, 2, 3, 4, 5, 6, 7, 8},
				})
				So(err, ShouldBeNil)

				_, err = api.GetGateway(ctx, &ns.GetGatewayRequest{
					Id: []byte{1, 2, 3, 4, 5, 6, 7, 8},
				})
				So(err, ShouldResemble, grpc.Errorf(codes.NotFound, "object does not exist"))
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
					start, _ := ptypes.TimestampProto(now.Truncate(time.Minute))
					end, _ := ptypes.TimestampProto(now)
					nowTrunc, _ := ptypes.TimestampProto(now.Truncate(time.Minute))

					resp, err := api.GetGatewayStats(ctx, &ns.GetGatewayStatsRequest{
						GatewayId:      []byte{1, 2, 3, 4, 5, 6, 7, 8},
						Interval:       ns.AggregationInterval_MINUTE,
						StartTimestamp: start,
						EndTimestamp:   end,
					})
					So(err, ShouldBeNil)
					So(resp.Result, ShouldHaveLength, 1)
					So(resp.Result[0].Timestamp, ShouldResemble, nowTrunc)
					So(resp.Result[0].RxPacketsReceived, ShouldEqual, 10)
					So(resp.Result[0].RxPacketsReceivedOk, ShouldEqual, 5)
					So(resp.Result[0].TxPacketsReceived, ShouldEqual, 11)
					So(resp.Result[0].TxPacketsEmitted, ShouldEqual, 10)
				})
			})

			Convey("When creating a gateway-profile object", func() {
				req := ns.CreateGatewayProfileRequest{
					GatewayProfile: &ns.GatewayProfile{
						Channels: []uint32{0, 1, 2},
						ExtraChannels: []*ns.GatewayProfileExtraChannel{
							{
								Modulation:       commonPB.Modulation_LORA,
								Frequency:        868700000,
								Bandwidth:        125,
								SpreadingFactors: []uint32{10, 11, 12},
							},
							{
								Modulation: commonPB.Modulation_FSK,
								Frequency:  868900000,
								Bandwidth:  125,
								Bitrate:    50000,
							},
						},
					},
				}
				createResp, err := api.CreateGatewayProfile(ctx, &req)
				So(err, ShouldBeNil)
				So(createResp.Id, ShouldHaveLength, 16)
				So(createResp.Id, ShouldNotResemble, uuid.Nil[:])

				Convey("Then it can be retrieved", func() {
					req.GatewayProfile.Id = createResp.Id

					getResp, err := api.GetGatewayProfile(ctx, &ns.GetGatewayProfileRequest{
						Id: createResp.Id,
					})
					So(err, ShouldBeNil)
					So(getResp.GatewayProfile, ShouldResemble, &ns.GatewayProfile{
						Id:       createResp.Id,
						Channels: []uint32{0, 1, 2},
						ExtraChannels: []*ns.GatewayProfileExtraChannel{
							{
								Modulation:       commonPB.Modulation_LORA,
								Frequency:        868700000,
								Bandwidth:        125,
								SpreadingFactors: []uint32{10, 11, 12},
							},
							{
								Modulation: commonPB.Modulation_FSK,
								Frequency:  868900000,
								Bandwidth:  125,
								Bitrate:    50000,
							},
						},
					})
				})

				Convey("Then it can be updated", func() {
					updateReq := ns.UpdateGatewayProfileRequest{
						GatewayProfile: &ns.GatewayProfile{
							Id:       createResp.Id,
							Channels: []uint32{0, 1},
							ExtraChannels: []*ns.GatewayProfileExtraChannel{
								{
									Modulation: commonPB.Modulation_FSK,
									Frequency:  868900000,
									Bandwidth:  125,
									Bitrate:    50000,
								},
								{
									Modulation:       commonPB.Modulation_LORA,
									Frequency:        868700000,
									Bandwidth:        125,
									SpreadingFactors: []uint32{10, 11, 12},
								},
							},
						},
					}
					_, err := api.UpdateGatewayProfile(ctx, &updateReq)
					So(err, ShouldBeNil)

					resp, err := api.GetGatewayProfile(ctx, &ns.GetGatewayProfileRequest{
						Id: createResp.Id,
					})
					So(err, ShouldBeNil)
					So(resp.GatewayProfile, ShouldResemble, &ns.GatewayProfile{
						Id:       createResp.Id,
						Channels: []uint32{0, 1},
						ExtraChannels: []*ns.GatewayProfileExtraChannel{
							{
								Modulation: commonPB.Modulation_FSK,
								Frequency:  868900000,
								Bandwidth:  125,
								Bitrate:    50000,
							},
							{
								Modulation:       commonPB.Modulation_LORA,
								Frequency:        868700000,
								Bandwidth:        125,
								SpreadingFactors: []uint32{10, 11, 12},
							},
						},
					})
				})

				Convey("Then it can be deleted", func() {
					_, err := api.DeleteGatewayProfile(ctx, &ns.DeleteGatewayProfileRequest{
						Id: createResp.Id,
					})
					So(err, ShouldBeNil)

					_, err = api.DeleteGatewayProfile(ctx, &ns.DeleteGatewayProfileRequest{
						Id: createResp.Id,
					})
					So(err, ShouldNotBeNil)
					So(grpc.Code(err), ShouldEqual, codes.NotFound)
				})
			})

			Convey("Then GetVersion returns the expected value", func() {
				config.Version = "1.2.3"

				resp, err := api.GetVersion(ctx, &empty.Empty{})
				So(err, ShouldBeNil)
				So(resp, ShouldResemble, &ns.GetVersionResponse{
					Version: "1.2.3",
					Region:  commonPB.Region_EU868,
				})
			})
		})
	})
}
