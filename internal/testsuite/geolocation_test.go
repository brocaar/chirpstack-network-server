package testsuite

import (
	"encoding/binary"
	"sort"
	"testing"
	"time"

	"github.com/brocaar/loraserver/internal/helpers"
	"github.com/brocaar/loraserver/internal/test"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/timestamp"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/brocaar/loraserver/api/as"
	"github.com/brocaar/loraserver/api/common"
	"github.com/brocaar/loraserver/api/geo"
	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/loraserver/internal/uplink"
	"github.com/brocaar/lorawan"
)

type GeolocationTestSuite struct {
	UplinkIntegrationTestSuite
}

func (ts *GeolocationTestSuite) SetupTest() {
	ts.UplinkIntegrationTestSuite.SetupTest()

	ts.CreateGateway(storage.Gateway{GatewayID: lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 1}, Location: storage.GPSPoint{Latitude: 1.1, Longitude: 1.1}, Altitude: 1.1})
	ts.CreateGateway(storage.Gateway{GatewayID: lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 2}, Location: storage.GPSPoint{Latitude: 2.1, Longitude: 2.1}, Altitude: 2.1})
	ts.CreateGateway(storage.Gateway{GatewayID: lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 3}, Location: storage.GPSPoint{Latitude: 3.1, Longitude: 3.1}, Altitude: 3.1})
	ts.CreateDevice(storage.Device{DevEUI: lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}})
	ts.CreateDeviceSession(storage.DeviceSession{})
}

func (ts *GeolocationTestSuite) TestGeolocation() {
	tests := []struct {
		Name                             string
		NwkGeoLoc                        bool
		RXInfo                           []*gw.UplinkRXInfo
		ResolveTDOAResponse              geo.ResolveTDOAResponse
		ExpectedGeolocationRequest       *geo.ResolveTDOARequest
		ExpectedSetDeviceLocationRequest *as.SetDeviceLocationRequest
	}{
		{
			Name:      "one gateway",
			NwkGeoLoc: true,
			RXInfo: []*gw.UplinkRXInfo{
				{
					GatewayId:         []byte{1, 1, 1, 1, 1, 1, 1, 1},
					FineTimestampType: gw.FineTimestampType_PLAIN,
					FineTimestamp: &gw.UplinkRXInfo_PlainFineTimestamp{
						PlainFineTimestamp: &gw.PlainFineTimestamp{
							Time: &timestamp.Timestamp{
								Nanos: 1,
							},
						},
					},
				},
			},
		},
		{
			Name:      "two gateways",
			NwkGeoLoc: true,
			RXInfo: []*gw.UplinkRXInfo{
				{
					GatewayId:         []byte{1, 1, 1, 1, 1, 1, 1, 1},
					FineTimestampType: gw.FineTimestampType_PLAIN,
					FineTimestamp: &gw.UplinkRXInfo_PlainFineTimestamp{
						PlainFineTimestamp: &gw.PlainFineTimestamp{
							Time: &timestamp.Timestamp{
								Nanos: 1,
							},
						},
					},
				},
				{
					GatewayId:         []byte{1, 1, 1, 1, 1, 1, 1, 2},
					FineTimestampType: gw.FineTimestampType_PLAIN,
					FineTimestamp: &gw.UplinkRXInfo_PlainFineTimestamp{
						PlainFineTimestamp: &gw.PlainFineTimestamp{
							Time: &timestamp.Timestamp{
								Nanos: 2,
							},
						},
					},
				},
			},
		},
		{
			Name:      "three gateways",
			NwkGeoLoc: true,
			RXInfo: []*gw.UplinkRXInfo{
				{
					GatewayId:         []byte{1, 1, 1, 1, 1, 1, 1, 1},
					FineTimestampType: gw.FineTimestampType_PLAIN,
					FineTimestamp: &gw.UplinkRXInfo_PlainFineTimestamp{
						PlainFineTimestamp: &gw.PlainFineTimestamp{
							Time: &timestamp.Timestamp{
								Nanos: 1,
							},
						},
					},
				},
				{
					GatewayId:         []byte{1, 1, 1, 1, 1, 1, 1, 2},
					FineTimestampType: gw.FineTimestampType_PLAIN,
					FineTimestamp: &gw.UplinkRXInfo_PlainFineTimestamp{
						PlainFineTimestamp: &gw.PlainFineTimestamp{
							Time: &timestamp.Timestamp{
								Nanos: 2,
							},
						},
					},
				},
				{
					GatewayId:         []byte{1, 1, 1, 1, 1, 1, 1, 3},
					FineTimestampType: gw.FineTimestampType_PLAIN,
					FineTimestamp: &gw.UplinkRXInfo_PlainFineTimestamp{
						PlainFineTimestamp: &gw.PlainFineTimestamp{
							Time: &timestamp.Timestamp{
								Nanos: 3,
							},
						},
					},
				},
			},
			ResolveTDOAResponse: geo.ResolveTDOAResponse{
				Result: &geo.ResolverResult{
					Location: &common.Location{
						Latitude:  1.123,
						Longitude: 2.123,
						Altitude:  3.123,
						Source:    common.LocationSource_GEO_RESOLVER,
					},
				},
			},
			ExpectedGeolocationRequest: &geo.ResolveTDOARequest{
				DevEui: ts.DeviceSession.DevEUI[:],
				FrameRxInfo: &geo.FrameRXInfo{
					RxInfo: []*gw.UplinkRXInfo{
						{
							GatewayId: []byte{1, 1, 1, 1, 1, 1, 1, 1},
							Location: &common.Location{
								Latitude:  1.1,
								Longitude: 1.1,
								Altitude:  1.1,
							},
							FineTimestampType: gw.FineTimestampType_PLAIN,
							FineTimestamp: &gw.UplinkRXInfo_PlainFineTimestamp{
								PlainFineTimestamp: &gw.PlainFineTimestamp{
									Time: &timestamp.Timestamp{
										Nanos: 1,
									},
								},
							},
						},
						{
							GatewayId: []byte{1, 1, 1, 1, 1, 1, 1, 2},
							Location: &common.Location{
								Latitude:  2.1,
								Longitude: 2.1,
								Altitude:  2.1,
							},
							FineTimestampType: gw.FineTimestampType_PLAIN,
							FineTimestamp: &gw.UplinkRXInfo_PlainFineTimestamp{
								PlainFineTimestamp: &gw.PlainFineTimestamp{
									Time: &timestamp.Timestamp{
										Nanos: 2,
									},
								},
							},
						},
						{
							GatewayId: []byte{1, 1, 1, 1, 1, 1, 1, 3},
							Location: &common.Location{
								Latitude:  3.1,
								Longitude: 3.1,
								Altitude:  3.1,
							},
							FineTimestampType: gw.FineTimestampType_PLAIN,
							FineTimestamp: &gw.UplinkRXInfo_PlainFineTimestamp{
								PlainFineTimestamp: &gw.PlainFineTimestamp{
									Time: &timestamp.Timestamp{
										Nanos: 3,
									},
								},
							},
						},
					},
				},
			},
			ExpectedSetDeviceLocationRequest: &as.SetDeviceLocationRequest{
				DevEui: ts.DeviceSession.DevEUI[:],
				Location: &common.Location{
					Latitude:  1.123,
					Longitude: 2.123,
					Altitude:  3.123,
					Source:    common.LocationSource_GEO_RESOLVER,
				},
			},
		},
		{
			Name:      "three gateways NwkGeoLoc disabled",
			NwkGeoLoc: false,
			RXInfo: []*gw.UplinkRXInfo{
				{
					GatewayId:         []byte{1, 1, 1, 1, 1, 1, 1, 1},
					FineTimestampType: gw.FineTimestampType_PLAIN,
					FineTimestamp: &gw.UplinkRXInfo_PlainFineTimestamp{
						PlainFineTimestamp: &gw.PlainFineTimestamp{
							Time: &timestamp.Timestamp{
								Nanos: 1,
							},
						},
					},
				},
				{
					GatewayId:         []byte{1, 1, 1, 1, 1, 1, 1, 2},
					FineTimestampType: gw.FineTimestampType_PLAIN,
					FineTimestamp: &gw.UplinkRXInfo_PlainFineTimestamp{
						PlainFineTimestamp: &gw.PlainFineTimestamp{
							Time: &timestamp.Timestamp{
								Nanos: 2,
							},
						},
					},
				},
				{
					GatewayId:         []byte{1, 1, 1, 1, 1, 1, 1, 3},
					FineTimestampType: gw.FineTimestampType_PLAIN,
					FineTimestamp: &gw.UplinkRXInfo_PlainFineTimestamp{
						PlainFineTimestamp: &gw.PlainFineTimestamp{
							Time: &timestamp.Timestamp{
								Nanos: 3,
							},
						},
					},
				},
			},
			ResolveTDOAResponse: geo.ResolveTDOAResponse{
				Result: &geo.ResolverResult{
					Location: &common.Location{
						Latitude:  1.123,
						Longitude: 2.123,
						Altitude:  3.123,
						Source:    common.LocationSource_GEO_RESOLVER,
					},
				},
			},
		},
		{
			Name:      "three gateways not enough fine-timestamp data",
			NwkGeoLoc: true,
			RXInfo: []*gw.UplinkRXInfo{
				{
					GatewayId:         []byte{1, 1, 1, 1, 1, 1, 1, 1},
					FineTimestampType: gw.FineTimestampType_PLAIN,
					FineTimestamp: &gw.UplinkRXInfo_PlainFineTimestamp{
						PlainFineTimestamp: &gw.PlainFineTimestamp{
							Time: &timestamp.Timestamp{
								Nanos: 1,
							},
						},
					},
				},
				{
					GatewayId:         []byte{1, 1, 1, 1, 1, 1, 1, 2},
					FineTimestampType: gw.FineTimestampType_PLAIN,
					FineTimestamp: &gw.UplinkRXInfo_PlainFineTimestamp{
						PlainFineTimestamp: &gw.PlainFineTimestamp{
							Time: &timestamp.Timestamp{
								Nanos: 2,
							},
						},
					},
				},
				{
					GatewayId: []byte{1, 1, 1, 1, 1, 1, 1, 3},
				},
			},
			ResolveTDOAResponse: geo.ResolveTDOAResponse{
				Result: &geo.ResolverResult{
					Location: &common.Location{
						Latitude:  1.123,
						Longitude: 2.123,
						Altitude:  3.123,
						Source:    common.LocationSource_GEO_RESOLVER,
					},
				},
			},
		},
	}

	for i, tst := range tests {
		ts.T().Run(tst.Name, func(t *testing.T) {
			assert := require.New(t)

			ts.FlushClients()
			test.MustFlushRedis(config.C.Redis.Pool)
			ts.GeoClient.ResolveTDOAResponse = tst.ResolveTDOAResponse

			ts.ServiceProfile.NwkGeoLoc = tst.NwkGeoLoc
			assert.NoError(storage.UpdateServiceProfile(config.C.PostgreSQL.DB, ts.ServiceProfile))
			ts.DeviceSession.FCntUp = uint32(i)
			assert.NoError(storage.SaveDeviceSession(config.C.Redis.Pool, *ts.DeviceSession))

			txInfo := gw.UplinkTXInfo{
				Frequency: 868100000,
			}
			assert.NoError(helpers.SetUplinkTXInfoDataRate(&txInfo, 3, config.C.NetworkServer.Band.Band))

			for j := range tst.RXInfo {
				uf := ts.GetUplinkFrameForFRMPayload(*tst.RXInfo[j], txInfo, lorawan.UnconfirmedDataUp, 10, []byte{1, 2, 3, 4})
				go func(uf gw.UplinkFrame) {
					assert.NoError(uplink.HandleRXPacket(uf))
				}(uf)
			}

			if tst.ExpectedGeolocationRequest != nil {
				geoReq := <-ts.GeoClient.ResolveTDOAChan
				sort.Sort(byGatewayID(geoReq.FrameRxInfo.RxInfo))
				if !proto.Equal(tst.ExpectedGeolocationRequest, &geoReq) {
					assert.EqualValues(*tst.ExpectedGeolocationRequest, geoReq)
				}
			} else {
				time.Sleep(100 * time.Millisecond)
				select {
				case <-ts.GeoClient.ResolveTDOAChan:
					t.Fatal("unexpected geolocation client request")
				default:
				}
			}

			if tst.ExpectedSetDeviceLocationRequest != nil {
				setLocReq := <-ts.ASClient.SetDeviceLocationChan
				if !proto.Equal(tst.ExpectedSetDeviceLocationRequest, &setLocReq) {
					assert.EqualValues(*tst.ExpectedSetDeviceLocationRequest, setLocReq)
				}
			} else {
				time.Sleep(100 * time.Millisecond)
				select {
				case <-ts.ASClient.SetDeviceLocationChan:
					t.Fatal("unexpected as client request")
				default:
				}
			}
		})
	}
}

func TestGeolocation(t *testing.T) {
	suite.Run(t, new(GeolocationTestSuite))
}

type byGatewayID []*gw.UplinkRXInfo

func (s byGatewayID) Len() int {
	return len(s)
}

func (s byGatewayID) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s byGatewayID) Less(i, j int) bool {
	ii := binary.BigEndian.Uint64(s[i].GatewayId)
	jj := binary.BigEndian.Uint64(s[j].GatewayId)
	return ii < jj
}
