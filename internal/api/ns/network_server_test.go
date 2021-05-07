package ns

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/gofrs/uuid"
	"github.com/golang/protobuf/ptypes/empty"
	. "github.com/smartystreets/goconvey/convey"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/brocaar/chirpstack-api/go/v3/common"
	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/brocaar/chirpstack-api/go/v3/ns"
	"github.com/brocaar/chirpstack-network-server/v3/internal/config"
	"github.com/brocaar/chirpstack-network-server/v3/internal/framelog"
	"github.com/brocaar/chirpstack-network-server/v3/internal/gps"
	"github.com/brocaar/chirpstack-network-server/v3/internal/storage"
	"github.com/brocaar/chirpstack-network-server/v3/internal/test"
	"github.com/brocaar/lorawan"
)

func TestNetworkServerAPI(t *testing.T) {
	conf := test.GetConfig()
	if err := storage.Setup(conf); err != nil {
		panic(err)
	}
	config.C.NetworkServer.NetID = [3]byte{1, 2, 3}

	Convey("Given a clean PostgreSQL and Redis database + api instance", t, func() {
		So(storage.MigrateDown(storage.DB().DB), ShouldBeNil)
		So(storage.MigrateUp(storage.DB().DB), ShouldBeNil)
		storage.RedisClient().FlushAll()

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
				So(framelog.LogDownlinkFrameForGateway(context.Background(), ns.DownlinkFrameLog{
					GatewayId: mac[:],
					TxInfo:    &gw.DownlinkTXInfo{},
				}), ShouldBeNil)

				Convey("Then the frame-log was received by the client", func() {
					resp := <-respChan
					So(resp.GetDownlinkFrame(), ShouldNotBeNil)
					So(resp.GetUplinkFrameSet(), ShouldBeNil)
				})
			})

			Convey("When logging an uplink gateway frame", func() {
				So(framelog.LogUplinkFrameForGateways(context.Background(), ns.UplinkFrameLog{
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
				So(framelog.LogDownlinkFrameForDevEUI(context.Background(), devEUI, ns.DownlinkFrameLog{}), ShouldBeNil)

				Convey("Then the frame-log was received by the client", func() {
					resp := <-respChan
					So(resp.GetDownlinkFrame(), ShouldNotBeNil)
					So(resp.GetUplinkFrameSet(), ShouldBeNil)
				})
			})

			Convey("When logging an uplink device frame", func() {
				So(framelog.LogUplinkFrameForDevEUI(context.Background(), devEUI, ns.UplinkFrameLog{}), ShouldBeNil)

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
					DrMin:                  5,
					DrMax:                  6,
					ChannelMask:            []byte{1, 2, 3},
					PrAllowed:              true,
					HrAllowed:              true,
					RaAllowed:              true,
					NwkGeoLoc:              true,
					TargetPer:              1,
					MinGwDiversity:         7,
					GwsPrivate:             true,
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
					DrMin:                  5,
					DrMax:                  6,
					ChannelMask:            []byte{1, 2, 3},
					PrAllowed:              true,
					HrAllowed:              true,
					RaAllowed:              true,
					NwkGeoLoc:              true,
					TargetPer:              1,
					MinGwDiversity:         7,
					GwsPrivate:             true,
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
						DrMin:                  7,
						DrMax:                  8,
						ChannelMask:            []byte{3, 2, 1},
						PrAllowed:              false,
						HrAllowed:              false,
						RaAllowed:              false,
						NwkGeoLoc:              false,
						TargetPer:              2,
						MinGwDiversity:         8,
						GwsPrivate:             false,
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
					DrMin:                  7,
					DrMax:                  8,
					ChannelMask:            []byte{3, 2, 1},
					PrAllowed:              false,
					HrAllowed:              false,
					RaAllowed:              false,
					NwkGeoLoc:              false,
					TargetPer:              2,
					MinGwDiversity:         8,
					GwsPrivate:             false,
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
					AdrAlgorithmId:     "default",
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
					AdrAlgorithmId:     "default",
				})
			})
		})

		Convey("Given a ServiceProfile, RoutingProfile, DeviceProfile and Device", func() {
			sp := storage.ServiceProfile{
				DRMin: 3,
				DRMax: 6,
			}
			So(storage.CreateServiceProfile(context.Background(), storage.DB(), &sp), ShouldBeNil)

			rp := storage.RoutingProfile{}
			So(storage.CreateRoutingProfile(context.Background(), storage.DB(), &rp), ShouldBeNil)

			dp := storage.DeviceProfile{
				FactoryPresetFreqs: []uint32{
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
			So(storage.CreateDeviceProfile(context.Background(), storage.DB(), &dp), ShouldBeNil)

			d := storage.Device{
				DevEUI:           devEUI,
				DeviceProfileID:  dp.ID,
				RoutingProfileID: rp.ID,
				ServiceProfileID: sp.ID,
			}
			So(storage.CreateDevice(context.Background(), storage.DB(), &d), ShouldBeNil)

			ds := storage.DeviceSession{
				DevEUI: d.DevEUI,
			}
			So(storage.SaveDeviceSession(context.Background(), ds), ShouldBeNil)

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
					So(storage.UpdateDevice(context.Background(), storage.DB(), &d), ShouldBeNil)

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
						ds, err := storage.GetDeviceSession(context.Background(), devEUI)
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
							queue, err := storage.GetMACCommandQueueItems(context.Background(), devEUI)
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
				So(storage.UpdateDeviceProfile(context.Background(), storage.DB(), &dp), ShouldBeNil)

				ds := storage.DeviceSession{
					DevEUI:       d.DevEUI,
					BeaconLocked: true,
					PingSlotNb:   1,
				}
				So(storage.SaveDeviceSession(context.Background(), ds), ShouldBeNil)

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
						queueItems, err := storage.GetDeviceQueueItemsForDevEUI(context.Background(), storage.DB(), d.DevEUI)
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
							queueItems, err := storage.GetDeviceQueueItemsForDevEUI(context.Background(), storage.DB(), d.DevEUI)
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
						DevAddr:    ds.DevAddr[:],
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
						DevAddr:    ds.DevAddr[:],
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

		Convey("Then GetVersion returns the expected value", func() {
			config.Version = "1.2.3"

			resp, err := api.GetVersion(ctx, &empty.Empty{})
			So(err, ShouldBeNil)
			So(resp, ShouldResemble, &ns.GetVersionResponse{
				Version: "1.2.3",
				Region:  common.Region_EU868,
			})
		})
	})
}
