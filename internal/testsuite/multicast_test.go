package testsuite

import (
	"context"
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/brocaar/loraserver/api/common"
	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/api/nc"
	"github.com/brocaar/loraserver/internal/gps"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/lorawan"
)

type MulticastTestSuite struct {
	IntegrationTestSuite
}

func (ts *MulticastTestSuite) SetupSuite() {
	ts.IntegrationTestSuite.SetupSuite()

	ts.CreateGateway(storage.Gateway{
		GatewayID: lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 1},
	})

	ts.CreateMulticastGroup(storage.MulticastGroup{
		GroupType:      storage.MulticastGroupB,
		MCAddr:         lorawan.DevAddr{1, 2, 3, 4},
		MCNwkSKey:      lorawan.AES128Key{1, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8},
		DR:             3,
		Frequency:      868300000,
		PingSlotPeriod: 32,
		FCnt:           10,
	})

	ts.CreateDevice(storage.Device{
		DevEUI: lorawan.EUI64{2, 2, 2, 2, 2, 2, 2, 2},
	})

	assert := require.New(ts.T())
	assert.NoError(storage.SaveDeviceGatewayRXInfoSet(context.Background(), storage.RedisPool(), storage.DeviceGatewayRXInfoSet{
		DevEUI: ts.Device.DevEUI,
		DR:     3,
		Items: []storage.DeviceGatewayRXInfo{
			{
				GatewayID: ts.Gateway.GatewayID,
				RSSI:      50,
				LoRaSNR:   5,
			},
		},
	}))

	assert.NoError(storage.AddDeviceToMulticastGroup(context.Background(), storage.DB(), ts.Device.DevEUI, ts.MulticastGroup.ID))
}

func (ts *MulticastTestSuite) TestMulticast() {
	now := time.Now().Round(time.Second).Add(-time.Second)
	nowGPS := gps.Time(now).TimeSinceGPSEpoch()
	fPort2 := uint8(2)

	tests := []MulticastTest{
		{
			Name:           "nothing in queue",
			MulticastGroup: *ts.MulticastGroup,
			Assert: []Assertion{
				AssertNoDownlinkFrame,
			},
		},
		{
			Name:           "one item im queue",
			MulticastGroup: *ts.MulticastGroup,
			MulticastQueueItems: []storage.MulticastQueueItem{
				{
					ScheduleAt:              now,
					EmitAtTimeSinceGPSEpoch: &nowGPS,
					MulticastGroupID:        ts.MulticastGroup.ID,
					GatewayID:               ts.Gateway.GatewayID,
					FCnt:                    10,
					FPort:                   2,
					FRMPayload:              []byte{1, 2, 3, 4},
				},
			},
			Assert: []Assertion{
				AssertMulticastQueueItems([]storage.MulticastQueueItem{}),
				AssertDownlinkFrame(gw.DownlinkTXInfo{
					GatewayId:  ts.Gateway.GatewayID[:],
					Frequency:  uint32(ts.MulticastGroup.Frequency),
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							SpreadingFactor:       9,
							CodeRate:              "4/5",
							Bandwidth:             125,
							PolarizationInversion: true,
						},
					},
					Timing: gw.DownlinkTiming_GPS_EPOCH,
					TimingInfo: &gw.DownlinkTXInfo_GpsEpochTimingInfo{
						GpsEpochTimingInfo: &gw.GPSEpochTimingInfo{
							TimeSinceGpsEpoch: ptypes.DurationProto(nowGPS),
						},
					},
				}, lorawan.PHYPayload{
					MHDR: lorawan.MHDR{
						MType: lorawan.UnconfirmedDataDown,
						Major: lorawan.LoRaWANR1,
					},
					MACPayload: &lorawan.MACPayload{
						FHDR: lorawan.FHDR{
							DevAddr: ts.MulticastGroup.MCAddr,
							FCnt:    10,
						},
						FPort:      &fPort2,
						FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: []byte{1, 2, 3, 4}}},
					},
					MIC: lorawan.MIC{0x8, 0xb5, 0x29, 0xe8},
				}),
				AssertNCHandleDownlinkMetaDataRequest(nc.HandleDownlinkMetaDataRequest{
					MulticastGroupId:            ts.MulticastGroup.ID[:],
					MessageType:                 nc.MType_UNCONFIRMED_DATA_DOWN,
					PhyPayloadByteCount:         17,
					ApplicationPayloadByteCount: 4,
					TxInfo: &gw.DownlinkTXInfo{
						GatewayId:  ts.Gateway.GatewayID[:],
						Frequency:  uint32(ts.MulticastGroup.Frequency),
						Power:      14,
						Modulation: common.Modulation_LORA,
						ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
							LoraModulationInfo: &gw.LoRaModulationInfo{
								SpreadingFactor:       9,
								CodeRate:              "4/5",
								Bandwidth:             125,
								PolarizationInversion: true,
							},
						},
						Timing: gw.DownlinkTiming_GPS_EPOCH,
						TimingInfo: &gw.DownlinkTXInfo_GpsEpochTimingInfo{
							GpsEpochTimingInfo: &gw.GPSEpochTimingInfo{
								TimeSinceGpsEpoch: ptypes.DurationProto(nowGPS),
							},
						},
					},
				}),
			},
		},
		{
			Name:           "two items in queue",
			MulticastGroup: *ts.MulticastGroup,
			MulticastQueueItems: []storage.MulticastQueueItem{
				{
					ScheduleAt:              now,
					EmitAtTimeSinceGPSEpoch: &nowGPS,
					MulticastGroupID:        ts.MulticastGroup.ID,
					GatewayID:               ts.Gateway.GatewayID,
					FCnt:                    10,
					FPort:                   2,
					FRMPayload:              []byte{1, 2, 3, 4},
				},
				{
					ScheduleAt:              now,
					EmitAtTimeSinceGPSEpoch: &nowGPS,
					MulticastGroupID:        ts.MulticastGroup.ID,
					GatewayID:               ts.Gateway.GatewayID,
					FCnt:                    11,
					FPort:                   2,
					FRMPayload:              []byte{1, 2, 3, 4},
				},
			},
			Assert: []Assertion{
				AssertMulticastQueueItems([]storage.MulticastQueueItem{
					{
						ScheduleAt:              now,
						EmitAtTimeSinceGPSEpoch: &nowGPS,
						MulticastGroupID:        ts.MulticastGroup.ID,
						GatewayID:               ts.Gateway.GatewayID,
						FCnt:                    11,
						FPort:                   2,
						FRMPayload:              []byte{1, 2, 3, 4},
					},
				}),
				AssertDownlinkFrame(gw.DownlinkTXInfo{
					GatewayId:  ts.Gateway.GatewayID[:],
					Frequency:  uint32(ts.MulticastGroup.Frequency),
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							SpreadingFactor:       9,
							CodeRate:              "4/5",
							Bandwidth:             125,
							PolarizationInversion: true,
						},
					},
					Timing: gw.DownlinkTiming_GPS_EPOCH,
					TimingInfo: &gw.DownlinkTXInfo_GpsEpochTimingInfo{
						GpsEpochTimingInfo: &gw.GPSEpochTimingInfo{
							TimeSinceGpsEpoch: ptypes.DurationProto(nowGPS),
						},
					},
				}, lorawan.PHYPayload{
					MHDR: lorawan.MHDR{
						MType: lorawan.UnconfirmedDataDown,
						Major: lorawan.LoRaWANR1,
					},
					MACPayload: &lorawan.MACPayload{
						FHDR: lorawan.FHDR{
							DevAddr: ts.MulticastGroup.MCAddr,
							FCnt:    10,
						},
						FPort:      &fPort2,
						FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: []byte{1, 2, 3, 4}}},
					},
					MIC: lorawan.MIC{0x8, 0xb5, 0x29, 0xe8},
				}),
			},
		},
		{
			Name:           "item discarded becayse of payload size",
			MulticastGroup: *ts.MulticastGroup,
			MulticastQueueItems: []storage.MulticastQueueItem{
				{
					ScheduleAt:              now,
					EmitAtTimeSinceGPSEpoch: &nowGPS,
					MulticastGroupID:        ts.MulticastGroup.ID,
					GatewayID:               ts.Gateway.GatewayID,
					FCnt:                    10,
					FPort:                   2,
					FRMPayload:              make([]byte, 300),
				},
			},
			Assert: []Assertion{
				AssertNoDownlinkFrame,
				AssertMulticastQueueItems([]storage.MulticastQueueItem{}),
			},
		},
	}

	for _, tst := range tests {
		ts.T().Run(tst.Name, func(t *testing.T) {
			ts.AssertMulticastTest(t, tst)
		})
	}
}

func TestMulticast(t *testing.T) {
	suite.Run(t, new(MulticastTestSuite))
}
