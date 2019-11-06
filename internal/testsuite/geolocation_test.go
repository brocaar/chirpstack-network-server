package testsuite

import (
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/timestamp"

	"github.com/stretchr/testify/suite"

	"github.com/brocaar/chirpstack-api/go/as"
	"github.com/brocaar/chirpstack-api/go/common"
	"github.com/brocaar/chirpstack-api/go/geo"
	"github.com/brocaar/chirpstack-api/go/gw"
	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/brocaar/lorawan"
)

type GeolocationTestSuite struct {
	IntegrationTestSuite
}

func (ts *GeolocationTestSuite) SetupTest() {
	ts.IntegrationTestSuite.SetupTest()

	ts.CreateGateway(storage.Gateway{GatewayID: lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 1}, Location: storage.GPSPoint{Latitude: 1.1, Longitude: 1.1}, Altitude: 1.1})
	ts.CreateGateway(storage.Gateway{GatewayID: lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 2}, Location: storage.GPSPoint{Latitude: 2.1, Longitude: 2.1}, Altitude: 2.1})
	ts.CreateGateway(storage.Gateway{GatewayID: lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 3}, Location: storage.GPSPoint{Latitude: 3.1, Longitude: 3.1}, Altitude: 3.1})
	ts.CreateDevice(storage.Device{DevEUI: lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}})
	ts.CreateDeviceSession(storage.DeviceSession{ReferenceAltitude: 5.6})
}

func (ts *GeolocationTestSuite) TestGeolocation() {
	now := time.Now()
	nowProto, _ := ptypes.TimestampProto(now)

	tests := []GeolocationTest{
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
			Assert: []Assertion{
				AssertNoASSetDeviceLocationRequest(),
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
			Assert: []Assertion{
				AssertNoASSetDeviceLocationRequest(),
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
				Result: &geo.ResolveResult{
					Location: &common.Location{
						Latitude:  1.123,
						Longitude: 2.123,
						Altitude:  3.123,
						Source:    common.LocationSource_GEO_RESOLVER,
					},
				},
			},
			Assert: []Assertion{
				AssertResolveTDOARequest(geo.ResolveTDOARequest{
					DevEui:                  ts.DeviceSession.DevEUI[:],
					DeviceReferenceAltitude: 5.6,
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
				}),
				AssertASSetDeviceLocationRequest(as.SetDeviceLocationRequest{
					DevEui: ts.DeviceSession.DevEUI[:],
					Location: &common.Location{
						Latitude:  1.123,
						Longitude: 2.123,
						Altitude:  3.123,
						Source:    common.LocationSource_GEO_RESOLVER,
					},
				}),
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
				Result: &geo.ResolveResult{
					Location: &common.Location{
						Latitude:  1.123,
						Longitude: 2.123,
						Altitude:  3.123,
						Source:    common.LocationSource_GEO_RESOLVER,
					},
				},
			},
			Assert: []Assertion{
				AssertNoASSetDeviceLocationRequest(),
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
				Result: &geo.ResolveResult{
					Location: &common.Location{
						Latitude:  1.123,
						Longitude: 2.123,
						Altitude:  3.123,
						Source:    common.LocationSource_GEO_RESOLVER,
					},
				},
			},
			Assert: []Assertion{
				AssertNoASSetDeviceLocationRequest(),
			},
		},
		{
			Name:      "three gateways + required 2 frames in buffer (have 1)",
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
			ResolveMultiFrameTDOAResponse: geo.ResolveMultiFrameTDOAResponse{
				Result: &geo.ResolveResult{
					Location: &common.Location{
						Latitude:  1.123,
						Longitude: 2.123,
						Altitude:  3.123,
						Source:    common.LocationSource_GEO_RESOLVER,
					},
				},
			},
			GeolocBufferTTL:     time.Minute,
			GeolocMinBufferSize: 2,
			Assert: []Assertion{
				AssertNoASSetDeviceLocationRequest(),
			},
		},
		{
			Name:      "three gateways + required 2 frames in buffer (have 2)",
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
			GeolocBufferItems: []*geo.FrameRXInfo{
				{
					RxInfo: []*gw.UplinkRXInfo{
						{
							GatewayId: []byte{1, 1, 1, 1, 1, 1, 1, 1},
							Time:      nowProto,
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
							Time:      nowProto,
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
							Time:      nowProto,
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
			ResolveMultiFrameTDOAResponse: geo.ResolveMultiFrameTDOAResponse{
				Result: &geo.ResolveResult{
					Location: &common.Location{
						Latitude:  1.123,
						Longitude: 2.123,
						Altitude:  3.123,
						Source:    common.LocationSource_GEO_RESOLVER,
					},
				},
			},
			GeolocBufferTTL:     time.Minute,
			GeolocMinBufferSize: 2,
			Assert: []Assertion{
				AssertResolveMultiFrameTDOARequest(geo.ResolveMultiFrameTDOARequest{
					DevEui: ts.Device.DevEUI[:],
					FrameRxInfoSet: []*geo.FrameRXInfo{
						{
							RxInfo: []*gw.UplinkRXInfo{
								{
									GatewayId: []byte{1, 1, 1, 1, 1, 1, 1, 1},
									Time:      nowProto,
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
									Time:      nowProto,
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
									Time:      nowProto,
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
						{
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
					DeviceReferenceAltitude: 5.6,
				}),
				AssertASSetDeviceLocationRequest(as.SetDeviceLocationRequest{
					DevEui: ts.DeviceSession.DevEUI[:],
					Location: &common.Location{
						Latitude:  1.123,
						Longitude: 2.123,
						Altitude:  3.123,
						Source:    common.LocationSource_GEO_RESOLVER,
					},
				}),
			},
		},
	}

	for i, tst := range tests {
		ts.T().Run(tst.Name, func(t *testing.T) {
			ts.AssertGeolocationTest(t, uint32(i), tst)
		})
	}
}

func TestGeolocation(t *testing.T) {
	suite.Run(t, new(GeolocationTestSuite))
}
