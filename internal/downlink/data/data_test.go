package data

import (
	"context"
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes/duration"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/brocaar/chirpstack-api/go/v3/as"
	"github.com/brocaar/chirpstack-api/go/v3/common"
	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/brocaar/chirpstack-network-server/internal/backend/applicationserver"
	"github.com/brocaar/chirpstack-network-server/internal/band"
	"github.com/brocaar/chirpstack-network-server/internal/config"
	"github.com/brocaar/chirpstack-network-server/internal/gps"
	"github.com/brocaar/chirpstack-network-server/internal/models"
	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/brocaar/chirpstack-network-server/internal/test"
	"github.com/brocaar/lorawan"
	loraband "github.com/brocaar/lorawan/band"
)

type GetNextDeviceQueueItemTestSuite struct {
	suite.Suite

	asClient       *test.ApplicationClient
	device         storage.Device
	serviceProfile storage.ServiceProfile
	deviceProfile  storage.DeviceProfile
	routingProfile storage.RoutingProfile
}

func (ts *GetNextDeviceQueueItemTestSuite) SetupSuite() {
	assert := require.New(ts.T())
	conf := test.GetConfig()
	assert.NoError(storage.Setup(conf))
	test.MustResetDB(storage.DB().DB)

	ts.asClient = test.NewApplicationClient()
	applicationserver.SetPool(test.NewApplicationServerPool(ts.asClient))

	ts.deviceProfile = storage.DeviceProfile{
		SupportsClassB: true,
		ClassBTimeout:  60,
	}

	assert.NoError(storage.CreateServiceProfile(context.Background(), storage.DB(), &ts.serviceProfile))
	assert.NoError(storage.CreateDeviceProfile(context.Background(), storage.DB(), &ts.deviceProfile))
	assert.NoError(storage.CreateRoutingProfile(context.Background(), storage.DB(), &ts.routingProfile))

	ts.device = storage.Device{
		DevEUI:           lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
		ServiceProfileID: ts.serviceProfile.ID,
		DeviceProfileID:  ts.deviceProfile.ID,
		RoutingProfileID: ts.routingProfile.ID,
	}
	assert.NoError(storage.CreateDevice(context.Background(), storage.DB(), &ts.device))
}

func (ts *GetNextDeviceQueueItemTestSuite) TestGetNextDeviceQueueItem() {
	now := time.Now()
	timeSinceGPSEpochNow := gps.Time(now).TimeSinceGPSEpoch()
	timeSinceGPSEpochFuture := gps.Time(now.Add(time.Second)).TimeSinceGPSEpoch()

	tests := []struct {
		name                              string
		maxPayloadSize                    int
		deviceSession                     storage.DeviceSession
		deviceQueueItems                  []storage.DeviceQueueItem
		reEncryptDeviceQueueItemsResponse as.ReEncryptDeviceQueueItemsResponse

		expectedDeviceQueueItem                  *storage.DeviceQueueItem
		expectedMoreDeviceQueueItems             bool
		expectedHandleDownlinkACKRequest         *as.HandleDownlinkACKRequest
		expectedReEncryptDeviceQueueItemsRequest *as.ReEncryptDeviceQueueItemsRequest
	}{
		{
			name:           "empty queue",
			maxPayloadSize: 100,
			deviceSession: storage.DeviceSession{
				DevAddr:          lorawan.DevAddr{1, 2, 3, 4},
				DevEUI:           ts.device.DevEUI,
				ServiceProfileID: ts.serviceProfile.ID,
				DeviceProfileID:  ts.deviceProfile.ID,
				RoutingProfileID: ts.routingProfile.ID,
				MACVersion:       "1.0.3",
				NFCntDown:        10,
			},
			deviceQueueItems:        nil,
			expectedDeviceQueueItem: nil,
		},
		{
			name:           "LoRaWAN 1.0 frame-counter",
			maxPayloadSize: 100,
			deviceSession: storage.DeviceSession{
				DevAddr:          lorawan.DevAddr{1, 2, 3, 4},
				DevEUI:           ts.device.DevEUI,
				ServiceProfileID: ts.serviceProfile.ID,
				DeviceProfileID:  ts.deviceProfile.ID,
				RoutingProfileID: ts.routingProfile.ID,
				MACVersion:       "1.0.3",
				NFCntDown:        10,
			},
			deviceQueueItems: []storage.DeviceQueueItem{
				{
					DevAddr:    lorawan.DevAddr{1, 2, 3, 4},
					DevEUI:     ts.device.DevEUI,
					FRMPayload: []byte{1, 2, 3},
					FCnt:       10,
					FPort:      3,
				},
				{
					DevAddr:    lorawan.DevAddr{1, 2, 3, 4},
					DevEUI:     ts.device.DevEUI,
					FRMPayload: []byte{4, 5, 6},
					FCnt:       11,
					FPort:      3,
				},
			},
			expectedDeviceQueueItem: &storage.DeviceQueueItem{
				DevAddr:    lorawan.DevAddr{1, 2, 3, 4},
				DevEUI:     ts.device.DevEUI,
				FRMPayload: []byte{1, 2, 3},
				FCnt:       10,
				FPort:      3,
			},
			expectedMoreDeviceQueueItems: true,
		},
		{
			name:           "LoRaWAN 1.1 frame-counter",
			maxPayloadSize: 100,
			deviceSession: storage.DeviceSession{
				DevAddr:          lorawan.DevAddr{1, 2, 3, 4},
				DevEUI:           ts.device.DevEUI,
				ServiceProfileID: ts.serviceProfile.ID,
				DeviceProfileID:  ts.deviceProfile.ID,
				RoutingProfileID: ts.routingProfile.ID,
				MACVersion:       "1.1.0",
				AFCntDown:        10,
			},
			deviceQueueItems: []storage.DeviceQueueItem{
				{
					DevAddr:    lorawan.DevAddr{1, 2, 3, 4},
					DevEUI:     ts.device.DevEUI,
					FRMPayload: []byte{1, 2, 3},
					FCnt:       10,
					FPort:      3,
				},
				{
					DevAddr:    lorawan.DevAddr{1, 2, 3, 4},
					DevEUI:     ts.device.DevEUI,
					FRMPayload: []byte{4, 5, 6},
					FCnt:       11,
					FPort:      3,
				},
			},
			expectedDeviceQueueItem: &storage.DeviceQueueItem{
				DevAddr:    lorawan.DevAddr{1, 2, 3, 4},
				DevEUI:     ts.device.DevEUI,
				FRMPayload: []byte{1, 2, 3},
				FCnt:       10,
				FPort:      3,
			},
			expectedMoreDeviceQueueItems: true,
		},
		{
			name:           "Payload timed out",
			maxPayloadSize: 100,
			deviceSession: storage.DeviceSession{
				DevAddr:          lorawan.DevAddr{1, 2, 3, 4},
				DevEUI:           ts.device.DevEUI,
				ServiceProfileID: ts.serviceProfile.ID,
				DeviceProfileID:  ts.deviceProfile.ID,
				RoutingProfileID: ts.routingProfile.ID,
				MACVersion:       "1.0.2",
				NFCntDown:        10,
			},
			deviceQueueItems: []storage.DeviceQueueItem{
				{
					DevAddr:      lorawan.DevAddr{1, 2, 3, 4},
					DevEUI:       ts.device.DevEUI,
					FRMPayload:   []byte{1, 2, 3},
					FCnt:         10,
					FPort:        3,
					TimeoutAfter: &now,
				},
			},
			expectedDeviceQueueItem: nil,
			expectedHandleDownlinkACKRequest: &as.HandleDownlinkACKRequest{
				DevEui:       ts.device.DevEUI[:],
				FCnt:         10,
				Acknowledged: false,
			},
		},
		{
			name:           "Frame-counter out-of-sync, re-enqueue",
			maxPayloadSize: 100,
			deviceSession: storage.DeviceSession{
				DevAddr:          lorawan.DevAddr{1, 2, 3, 4},
				DevEUI:           ts.device.DevEUI,
				ServiceProfileID: ts.serviceProfile.ID,
				DeviceProfileID:  ts.deviceProfile.ID,
				RoutingProfileID: ts.routingProfile.ID,
				MACVersion:       "1.0.2",
				NFCntDown:        11,
			},
			deviceQueueItems: []storage.DeviceQueueItem{
				{
					DevAddr:    lorawan.DevAddr{1, 2, 3, 4},
					DevEUI:     ts.device.DevEUI,
					FRMPayload: []byte{1, 2, 3},
					FCnt:       10,
					FPort:      3,
				},
				{
					DevAddr:    lorawan.DevAddr{1, 2, 3, 4},
					DevEUI:     ts.device.DevEUI,
					FRMPayload: []byte{4, 5, 6},
					FCnt:       11,
					FPort:      3,
				},
			},
			reEncryptDeviceQueueItemsResponse: as.ReEncryptDeviceQueueItemsResponse{
				Items: []*as.ReEncryptedDeviceQueueItem{
					{
						FrmPayload: []byte{3, 2, 1},
						FCnt:       11,
						FPort:      3,
					},
					{
						FrmPayload: []byte{6, 5, 4},
						FCnt:       12,
						FPort:      3,
					},
				},
			},
			expectedReEncryptDeviceQueueItemsRequest: &as.ReEncryptDeviceQueueItemsRequest{
				DevEui:    ts.device.DevEUI[:],
				DevAddr:   []byte{1, 2, 3, 4},
				FCntStart: 11,
				Items: []*as.ReEncryptDeviceQueueItem{
					{
						FrmPayload: []byte{1, 2, 3},
						FCnt:       10,
						FPort:      3,
					},
					{
						FrmPayload: []byte{4, 5, 6},
						FCnt:       11,
						FPort:      3,
					},
				},
			},
			expectedMoreDeviceQueueItems: true,
			expectedDeviceQueueItem: &storage.DeviceQueueItem{
				DevAddr:    lorawan.DevAddr{1, 2, 3, 4},
				DevEUI:     ts.device.DevEUI,
				FRMPayload: []byte{3, 2, 1},
				FCnt:       11,
				FPort:      3,
			},
		},
		{
			name:           "First queue item dropped because of max. payload size, second re-synced",
			maxPayloadSize: 100,
			deviceSession: storage.DeviceSession{
				DevAddr:          lorawan.DevAddr{1, 2, 3, 4},
				DevEUI:           ts.device.DevEUI,
				ServiceProfileID: ts.serviceProfile.ID,
				DeviceProfileID:  ts.deviceProfile.ID,
				RoutingProfileID: ts.routingProfile.ID,
				MACVersion:       "1.0.2",
				NFCntDown:        10,
			},
			deviceQueueItems: []storage.DeviceQueueItem{
				{
					DevAddr:    lorawan.DevAddr{1, 2, 3, 4},
					DevEUI:     ts.device.DevEUI,
					FRMPayload: make([]byte, 101),
					FCnt:       10,
					FPort:      3,
				},
				{
					DevAddr:    lorawan.DevAddr{1, 2, 3, 4},
					DevEUI:     ts.device.DevEUI,
					FRMPayload: []byte{4, 5, 6},
					FCnt:       11,
					FPort:      3,
				},
			},
			reEncryptDeviceQueueItemsResponse: as.ReEncryptDeviceQueueItemsResponse{
				Items: []*as.ReEncryptedDeviceQueueItem{
					{
						FrmPayload: []byte{3, 2, 1},
						FCnt:       10,
						FPort:      3,
					},
				},
			},
			expectedReEncryptDeviceQueueItemsRequest: &as.ReEncryptDeviceQueueItemsRequest{
				DevEui:    ts.device.DevEUI[:],
				DevAddr:   []byte{1, 2, 3, 4},
				FCntStart: 10,
				Items: []*as.ReEncryptDeviceQueueItem{
					{
						FrmPayload: []byte{4, 5, 6},
						FCnt:       11,
						FPort:      3,
					},
				},
			},
			expectedDeviceQueueItem: &storage.DeviceQueueItem{
				DevAddr:    lorawan.DevAddr{1, 2, 3, 4},
				DevEUI:     ts.device.DevEUI,
				FRMPayload: []byte{3, 2, 1},
				FCnt:       10,
				FPort:      3,
			},
		},
		{
			name:           "Class-B emit_at_time_since_gps_epoch is in the past",
			maxPayloadSize: 100,
			deviceSession: storage.DeviceSession{
				DevAddr:          lorawan.DevAddr{1, 2, 3, 4},
				DevEUI:           ts.device.DevEUI,
				ServiceProfileID: ts.serviceProfile.ID,
				DeviceProfileID:  ts.deviceProfile.ID,
				RoutingProfileID: ts.routingProfile.ID,
				MACVersion:       "1.0.3",
				NFCntDown:        10,
				BeaconLocked:     true,
				PingSlotNb:       1,
			},
			deviceQueueItems: []storage.DeviceQueueItem{
				{
					DevAddr:                 lorawan.DevAddr{1, 2, 3, 4},
					DevEUI:                  ts.device.DevEUI,
					FRMPayload:              []byte{1, 2, 3},
					FCnt:                    10,
					FPort:                   3,
					EmitAtTimeSinceGPSEpoch: &timeSinceGPSEpochNow,
				},
			},
			expectedDeviceQueueItem: &storage.DeviceQueueItem{
				DevAddr:                 lorawan.DevAddr{1, 2, 3, 4},
				DevEUI:                  ts.device.DevEUI,
				FRMPayload:              []byte{1, 2, 3},
				FCnt:                    10,
				FPort:                   3,
				EmitAtTimeSinceGPSEpoch: &timeSinceGPSEpochFuture, // we will validate that this value > now
			},
			expectedMoreDeviceQueueItems: false,
		},
	}

	for _, tst := range tests {
		ts.T().Run(tst.name, func(t *testing.T) {
			assert := require.New(t)

			// set mock
			ts.asClient.ReEncryptDeviceQueueItemsResponse = tst.reEncryptDeviceQueueItemsResponse

			// Populate the device-queue
			assert.NoError(storage.FlushDeviceQueueForDevEUI(context.Background(), storage.DB(), ts.device.DevEUI))
			for _, item := range tst.deviceQueueItems {
				assert.NoError(storage.CreateDeviceQueueItem(context.Background(), storage.DB(), &item, ts.deviceProfile, tst.deviceSession))
			}

			// setup context
			ctx := dataContext{
				ctx:           context.Background(),
				DB:            storage.DB(),
				DeviceProfile: ts.deviceProfile,
				DeviceSession: tst.deviceSession,
				DownlinkFrameItems: []downlinkFrameItem{
					{
						RemainingPayloadSize: tst.maxPayloadSize,
						DownlinkFrameItem: gw.DownlinkFrameItem{
							TxInfo: &gw.DownlinkTXInfo{},
						},
					},
				},
			}

			assert.NoError(getNextDeviceQueueItem(&ctx))

			if tst.expectedDeviceQueueItem == nil {
				assert.Nil(ctx.DeviceQueueItem)
			} else {
				assert.NotNil(ctx.DeviceQueueItem)
				assert.Equal(tst.expectedDeviceQueueItem.FCnt, ctx.DeviceQueueItem.FCnt)
				assert.Equal(tst.expectedDeviceQueueItem.FPort, ctx.DeviceQueueItem.FPort)
				assert.Equal(tst.expectedDeviceQueueItem.FRMPayload, ctx.DeviceQueueItem.FRMPayload)

				if tst.expectedDeviceQueueItem.EmitAtTimeSinceGPSEpoch != nil {
					assert.NotNil(ctx.DeviceQueueItem.EmitAtTimeSinceGPSEpoch)
					assert.Greater(int64(*tst.expectedDeviceQueueItem.EmitAtTimeSinceGPSEpoch), int64(timeSinceGPSEpochNow))
				} else {
					assert.Nil(ctx.DeviceQueueItem.EmitAtTimeSinceGPSEpoch)
				}

				assert.Equal(tst.expectedMoreDeviceQueueItems, ctx.MoreDeviceQueueItems)
			}

			if tst.expectedHandleDownlinkACKRequest != nil {
				req := <-ts.asClient.HandleDownlinkACKChan
				assert.Equal(tst.expectedHandleDownlinkACKRequest, &req)
			}

			if tst.expectedReEncryptDeviceQueueItemsRequest != nil {
				req := <-ts.asClient.ReEncryptDeviceQueueItemsChan
				assert.Equal(tst.expectedReEncryptDeviceQueueItemsRequest, &req)
			}
		})
	}
}

func TestGetNextDeviceQueueItem(t *testing.T) {
	suite.Run(t, new(GetNextDeviceQueueItemTestSuite))
}

type SetMACCommandsSetTestSuite struct {
	suite.Suite
}

func (ts *SetMACCommandsSetTestSuite) SetupSuite() {
	assert := require.New(ts.T())
	conf := test.GetConfig()
	assert.NoError(storage.Setup(conf))
}

func (ts *SetMACCommandsSetTestSuite) TestSetMACCommandsSet() {
	tests := []struct {
		Name                string
		BeforeFunc          func() error
		DataContext         dataContext
		ExpectedMACCommands []storage.MACCommandBlock
	}{
		{
			Name: "trigger channel-reconfiguration",
			DataContext: dataContext{
				ServiceProfile: storage.ServiceProfile{
					DRMax: 5,
				},
				DeviceSession: storage.DeviceSession{
					EnabledUplinkChannels: []int{0, 1},
					TXPowerIndex:          2,
					DR:                    5,
					NbTrans:               2,
					RX2Frequency:          869525000,
				},
				DownlinkFrameItems: []downlinkFrameItem{
					{
						RemainingPayloadSize: 200,
					},
				},
			},
			ExpectedMACCommands: []storage.MACCommandBlock{
				{
					CID: lorawan.LinkADRReq,
					MACCommands: storage.MACCommands{
						{
							CID: lorawan.LinkADRReq,
							Payload: &lorawan.LinkADRReqPayload{
								DataRate: 5,
								TXPower:  2,
								ChMask:   [16]bool{true, true, true},
								Redundancy: lorawan.Redundancy{
									NbRep: 2,
								},
							},
						},
					},
				},
			},
		},
		{
			Name: "trigger channel-reconfiguration - exceed error count",
			DataContext: dataContext{
				ServiceProfile: storage.ServiceProfile{
					DRMax: 5,
				},
				DeviceSession: storage.DeviceSession{
					EnabledUplinkChannels: []int{0, 1},
					TXPowerIndex:          2,
					DR:                    5,
					NbTrans:               2,
					RX2Frequency:          869525000,
					MACCommandErrorCount: map[lorawan.CID]int{
						lorawan.LinkADRReq: 4, // 3 is the default max
					},
				},
				DownlinkFrameItems: []downlinkFrameItem{
					{
						RemainingPayloadSize: 200,
					},
				},
			},
			ExpectedMACCommands: nil,
		},
		{
			Name: "trigger adr request change",
			DataContext: dataContext{
				ServiceProfile: storage.ServiceProfile{
					DRMax: 5,
				},
				DeviceSession: storage.DeviceSession{
					ADR: true,
					DR:  0,
					UplinkHistory: []storage.UplinkHistory{
						{FCnt: 0, MaxSNR: 5, TXPowerIndex: 0, GatewayCount: 1},
					},
					RX2Frequency: 869525000,
				},
				DownlinkFrameItems: []downlinkFrameItem{
					{
						RemainingPayloadSize: 200,
					},
				},
			},
			ExpectedMACCommands: []storage.MACCommandBlock{
				{
					CID: lorawan.LinkADRReq,
					MACCommands: storage.MACCommands{
						{
							CID: lorawan.LinkADRReq,
							Payload: &lorawan.LinkADRReqPayload{
								DataRate: 5,
								TXPower:  3,
								ChMask:   [16]bool{true, true, true},
								Redundancy: lorawan.Redundancy{
									NbRep: 1,
								},
							},
						},
					},
				},
			},
		},
		{
			Name: "trigger adr request change + MaxSupportedTXPowerIndex",
			DataContext: dataContext{
				ServiceProfile: storage.ServiceProfile{
					DRMax: 5,
				},
				DeviceSession: storage.DeviceSession{
					ADR: true,
					DR:  0,
					UplinkHistory: []storage.UplinkHistory{
						{FCnt: 0, MaxSNR: 5, TXPowerIndex: 0, GatewayCount: 1},
					},
					RX2Frequency:             869525000,
					MaxSupportedTXPowerIndex: 2,
				},
				DownlinkFrameItems: []downlinkFrameItem{
					{
						RemainingPayloadSize: 200,
					},
				},
			},
			ExpectedMACCommands: []storage.MACCommandBlock{
				{
					CID: lorawan.LinkADRReq,
					MACCommands: storage.MACCommands{
						{
							CID: lorawan.LinkADRReq,
							Payload: &lorawan.LinkADRReqPayload{
								DataRate: 5,
								TXPower:  2,
								ChMask:   [16]bool{true, true, true},
								Redundancy: lorawan.Redundancy{
									NbRep: 1,
								},
							},
						},
					},
				},
			},
		},
		{
			Name: "request device-status",
			DataContext: dataContext{
				ServiceProfile: storage.ServiceProfile{
					DRMax:            5,
					DevStatusReqFreq: 1,
				},
				DeviceSession: storage.DeviceSession{
					EnabledUplinkChannels: []int{0, 1, 2},
					RX2Frequency:          869525000,
				},
				DownlinkFrameItems: []downlinkFrameItem{
					{
						RemainingPayloadSize: 200,
					},
				},
			},
			ExpectedMACCommands: []storage.MACCommandBlock{
				{
					CID: lorawan.DevStatusReq,
					MACCommands: storage.MACCommands{
						{
							CID: lorawan.DevStatusReq,
						},
					},
				},
			},
		},
		{
			BeforeFunc: func() error {
				conf := test.GetConfig()
				conf.NetworkServer.NetworkSettings.ClassB.PingSlotDR = 3
				conf.NetworkServer.NetworkSettings.ClassB.PingSlotFrequency = 868100000
				return Setup(conf)
			},
			Name: "trigger ping-slot parameters",
			DataContext: dataContext{
				ServiceProfile: storage.ServiceProfile{
					DRMax: 5,
				},
				DeviceProfile: storage.DeviceProfile{
					SupportsClassB: true,
				},
				DeviceSession: storage.DeviceSession{
					PingSlotDR:            2,
					PingSlotFrequency:     868300000,
					EnabledUplinkChannels: []int{0, 1, 2},
					RX2Frequency:          869525000,
				},
				DownlinkFrameItems: []downlinkFrameItem{
					{
						RemainingPayloadSize: 200,
					},
				},
			},
			ExpectedMACCommands: []storage.MACCommandBlock{
				{
					CID: lorawan.PingSlotChannelReq,
					MACCommands: storage.MACCommands{
						{
							CID: lorawan.PingSlotChannelReq,
							Payload: &lorawan.PingSlotChannelReqPayload{
								Frequency: 868100000,
								DR:        3,
							},
						},
					},
				},
			},
		},
		{
			Name: "trigger channel-mask reconfiguration",
			DataContext: dataContext{
				ServiceProfile: storage.ServiceProfile{
					DRMax: 5,
				},
				DeviceSession: storage.DeviceSession{
					EnabledUplinkChannels: []int{0, 1, 2, 3, 4, 5},
					ExtraUplinkChannels: map[int]loraband.Channel{
						3: loraband.Channel{},
						4: loraband.Channel{},
						6: loraband.Channel{},
					},
					DR:           5,
					TXPowerIndex: 3,
					RX2Frequency: 869525000,
				},
				DownlinkFrameItems: []downlinkFrameItem{
					{
						RemainingPayloadSize: 200,
					},
				},
			},
			ExpectedMACCommands: []storage.MACCommandBlock{
				{
					CID: lorawan.LinkADRReq,
					MACCommands: storage.MACCommands{
						{
							CID: lorawan.LinkADRReq,
							Payload: &lorawan.LinkADRReqPayload{
								DataRate: 5,
								TXPower:  3,
								ChMask:   lorawan.ChMask{true, true, true},
							},
						},
					},
				},
			},
		},
		{
			BeforeFunc: func() error {
				band.Band().AddChannel(868600000, 3, 5)
				band.Band().AddChannel(868700000, 4, 5)
				band.Band().AddChannel(868800000, 5, 5)
				return nil
			},
			Name: "trigger adding new channel",
			DataContext: dataContext{
				ServiceProfile: storage.ServiceProfile{
					DRMax: 5,
				},
				DeviceSession: storage.DeviceSession{
					EnabledUplinkChannels: []int{0, 1, 2},
					RX2Frequency:          869525000,
				},
				DownlinkFrameItems: []downlinkFrameItem{
					{
						RemainingPayloadSize: 200,
					},
				},
			},
			ExpectedMACCommands: []storage.MACCommandBlock{
				{
					CID: lorawan.NewChannelReq,
					MACCommands: storage.MACCommands{
						{
							CID: lorawan.NewChannelReq,
							Payload: &lorawan.NewChannelReqPayload{
								ChIndex: 3,
								Freq:    868600000,
								MinDR:   3,
								MaxDR:   5,
							},
						},
						{
							CID: lorawan.NewChannelReq,
							Payload: &lorawan.NewChannelReqPayload{
								ChIndex: 4,
								Freq:    868700000,
								MinDR:   4,
								MaxDR:   5,
							},
						},
						{
							CID: lorawan.NewChannelReq,
							Payload: &lorawan.NewChannelReqPayload{
								ChIndex: 5,
								Freq:    868800000,
								MinDR:   5,
								MaxDR:   5,
							},
						},
					},
				},
			},
		},
		{
			BeforeFunc: func() error {
				band.Band().AddChannel(868600000, 3, 5)
				band.Band().AddChannel(868700000, 4, 5)
				band.Band().AddChannel(868800000, 5, 5)
				return nil
			},
			Name: "trigger updating existing channels",
			DataContext: dataContext{
				ServiceProfile: storage.ServiceProfile{
					DRMax: 5,
				},
				DeviceSession: storage.DeviceSession{
					EnabledUplinkChannels: []int{0, 1, 2},
					ExtraUplinkChannels: map[int]loraband.Channel{
						3: loraband.Channel{Frequency: 868550000, MinDR: 3, MaxDR: 5},
						4: loraband.Channel{Frequency: 868700000, MinDR: 4, MaxDR: 5},
						5: loraband.Channel{Frequency: 868800000, MinDR: 4, MaxDR: 5},
					},
					RX2Frequency: 869525000,
				},
				DownlinkFrameItems: []downlinkFrameItem{
					{
						RemainingPayloadSize: 200,
					},
				},
			},
			ExpectedMACCommands: []storage.MACCommandBlock{
				{
					CID: lorawan.NewChannelReq,
					MACCommands: storage.MACCommands{
						{
							CID: lorawan.NewChannelReq,
							Payload: &lorawan.NewChannelReqPayload{
								ChIndex: 3,
								Freq:    868600000,
								MinDR:   3,
								MaxDR:   5,
							},
						},
						{
							CID: lorawan.NewChannelReq,
							Payload: &lorawan.NewChannelReqPayload{
								ChIndex: 5,
								Freq:    868800000,
								MinDR:   5,
								MaxDR:   5,
							},
						},
					},
				},
			},
		},
		{
			BeforeFunc: func() error {
				conf := test.GetConfig()
				conf.NetworkServer.NetworkSettings.RX2Frequency = 868700000
				conf.NetworkServer.NetworkSettings.RX2DR = 5
				conf.NetworkServer.NetworkSettings.RX1DROffset = 3
				return Setup(conf)
			},
			Name: "trigger rx param setup",
			DataContext: dataContext{
				ServiceProfile: storage.ServiceProfile{
					DRMax: 5,
				},
				DeviceSession: storage.DeviceSession{
					EnabledUplinkChannels: []int{0, 1, 2},
					RX2Frequency:          868100000,
					RX2DR:                 1,
					RX1DROffset:           0,
				},
				DownlinkFrameItems: []downlinkFrameItem{
					{
						RemainingPayloadSize: 200,
					},
				},
			},
			ExpectedMACCommands: []storage.MACCommandBlock{
				{
					CID: lorawan.RXParamSetupReq,
					MACCommands: storage.MACCommands{
						{
							CID: lorawan.RXParamSetupReq,
							Payload: &lorawan.RXParamSetupReqPayload{
								Frequency: 868700000,
								DLSettings: lorawan.DLSettings{
									RX2DataRate: 5,
									RX1DROffset: 3,
								},
							},
						},
					},
				},
			},
		},
		{
			BeforeFunc: func() error {
				conf := test.GetConfig()
				conf.NetworkServer.NetworkSettings.RX1Delay = 14
				return Setup(conf)
			},
			Name: "trigger rx timing setup",
			DataContext: dataContext{
				ServiceProfile: storage.ServiceProfile{
					DRMax: 5,
				},
				DeviceSession: storage.DeviceSession{
					EnabledUplinkChannels: []int{0, 1, 2},
					RX2Frequency:          869525000,
					RXDelay:               1,
				},
				DownlinkFrameItems: []downlinkFrameItem{
					{
						RemainingPayloadSize: 200,
					},
				},
			},
			ExpectedMACCommands: []storage.MACCommandBlock{
				{
					CID: lorawan.RXTimingSetupReq,
					MACCommands: []lorawan.MACCommand{
						{
							CID: lorawan.RXTimingSetupReq,
							Payload: &lorawan.RXTimingSetupReqPayload{
								Delay: 14,
							},
						},
					},
				},
			},
		},
		{
			// This tests that in case a LinkADRReq -and- a NewChannelReq
			// is requested, the LinkADRReq is dropped.
			// The reason is that the NewChannelReq mac-commands adds a new
			// channel, which can only be added to the channelmask after
			// an ACK from the device. Without this, the following would happen:
			// NewChannelReq asks to add channel C
			// LinkADRReq only enables A, B and disables C (as it does not
			// know about the new channel C yet).
			BeforeFunc: func() error {
				return band.Band().AddChannel(868300000, 6, 6)
			},
			Name: "LinkADRReq and NewChannelReq requested at the same time (will drop LinkADRReq)",
			DataContext: dataContext{
				ServiceProfile: storage.ServiceProfile{
					DRMax: 5,
				},
				DeviceSession: storage.DeviceSession{
					ADR: true,
					DR:  0,
					UplinkHistory: []storage.UplinkHistory{
						{FCnt: 0, MaxSNR: 5, TXPowerIndex: 0, GatewayCount: 1},
					},
					RX2Frequency: 869525000,
				},
				DownlinkFrameItems: []downlinkFrameItem{
					{
						RemainingPayloadSize: 200,
					},
				},
			},
			ExpectedMACCommands: []storage.MACCommandBlock{
				{
					CID: lorawan.NewChannelReq,
					MACCommands: storage.MACCommands{
						{
							CID: lorawan.NewChannelReq,
							Payload: &lorawan.NewChannelReqPayload{
								ChIndex: 3,
								Freq:    868300000,
								MaxDR:   6,
								MinDR:   6,
							},
						},
					},
				},
			},
		},
		{
			BeforeFunc: func() error {
				conf := test.GetConfig()
				conf.NetworkServer.NetworkSettings.RejoinRequest.Enabled = true
				conf.NetworkServer.NetworkSettings.RejoinRequest.MaxCountN = 1
				conf.NetworkServer.NetworkSettings.RejoinRequest.MaxTimeN = 2
				return Setup(conf)
			},
			Name: "trigger rejoin param setup request",
			DataContext: dataContext{
				ServiceProfile: storage.ServiceProfile{
					DRMax: 5,
				},
				DeviceSession: storage.DeviceSession{
					EnabledUplinkChannels: []int{0, 1, 2},
					TXPowerIndex:          2,
					DR:                    5,
					NbTrans:               2,
					RX2Frequency:          869525000,
					MACVersion:            "1.1.0",
				},
				DownlinkFrameItems: []downlinkFrameItem{
					{
						RemainingPayloadSize: 200,
					},
				},
			},
			ExpectedMACCommands: []storage.MACCommandBlock{
				{
					CID: lorawan.RejoinParamSetupReq,
					MACCommands: []lorawan.MACCommand{
						{
							CID: lorawan.RejoinParamSetupReq,
							Payload: &lorawan.RejoinParamSetupReqPayload{
								MaxCountN: 1,
								MaxTimeN:  2,
							},
						},
					},
				},
			},
		},
		{
			BeforeFunc: func() error {
				conf := test.GetConfig()
				conf.NetworkServer.NetworkSettings.RejoinRequest.Enabled = true
				conf.NetworkServer.NetworkSettings.RejoinRequest.MaxCountN = 1
				conf.NetworkServer.NetworkSettings.RejoinRequest.MaxTimeN = 2
				return Setup(conf)
			},
			Name: "trigger rejoin param setup request (ignored because of LoRaWAN 1.0)",
			DataContext: dataContext{
				ServiceProfile: storage.ServiceProfile{
					DRMax: 5,
				},
				DeviceSession: storage.DeviceSession{
					EnabledUplinkChannels: []int{0, 1, 2},
					TXPowerIndex:          2,
					DR:                    5,
					NbTrans:               2,
					RX2Frequency:          869525000,
					MACVersion:            "1.0.2",
				},
				DownlinkFrameItems: []downlinkFrameItem{
					{
						RemainingPayloadSize: 200,
					},
				},
			},
		},
		{
			BeforeFunc: func() error {
				conf := test.GetConfig()
				conf.NetworkServer.NetworkSettings.RejoinRequest.Enabled = true
				conf.NetworkServer.NetworkSettings.RejoinRequest.MaxCountN = 1
				conf.NetworkServer.NetworkSettings.RejoinRequest.MaxTimeN = 2
				return Setup(conf)
			},
			Name: "trigger rejoin param setup request are in sync",
			DataContext: dataContext{
				ServiceProfile: storage.ServiceProfile{
					DRMax: 5,
				},
				DeviceSession: storage.DeviceSession{
					EnabledUplinkChannels:  []int{0, 1, 2},
					TXPowerIndex:           2,
					DR:                     5,
					NbTrans:                2,
					RX2Frequency:           869525000,
					RejoinRequestEnabled:   true,
					RejoinRequestMaxCountN: 1,
					RejoinRequestMaxTimeN:  2,
				},
				DownlinkFrameItems: []downlinkFrameItem{
					{
						RemainingPayloadSize: 200,
					},
				},
			},
		},
	}

	for _, tst := range tests {
		ts.T().Run(tst.Name, func(t *testing.T) {
			assert := require.New(t)
			conf := test.GetConfig()
			assert.NoError(Setup(conf))
			assert.NoError(band.Setup(conf))

			storage.RedisClient().FlushAll()

			if tst.BeforeFunc != nil {
				assert.NoError(tst.BeforeFunc())
			}

			tst.DataContext.ctx = context.Background()

			assert.NoError(setMACCommandsSet(&tst.DataContext))
			assert.Equal(tst.ExpectedMACCommands, tst.DataContext.MACCommands)
		})
	}
}

func TestSetMACCommandsSet(t *testing.T) {
	suite.Run(t, new(SetMACCommandsSetTestSuite))
}

func TestFilterIncompatibleMACCommands(t *testing.T) {
	tests := []struct {
		Name        string
		MACCommands []storage.MACCommandBlock
		Expected    []storage.MACCommandBlock
	}{
		{
			Name: "LinkADRReq",
			MACCommands: []storage.MACCommandBlock{
				{CID: lorawan.LinkADRReq},
			},
			Expected: []storage.MACCommandBlock{
				{CID: lorawan.LinkADRReq},
			},
		},
		{
			Name: "NewChannelReq",
			MACCommands: []storage.MACCommandBlock{
				{CID: lorawan.NewChannelReq},
			},
			Expected: []storage.MACCommandBlock{
				{CID: lorawan.NewChannelReq},
			},
		},
		{
			Name: "NewChannelReq + LinkADRReq",
			MACCommands: []storage.MACCommandBlock{
				{CID: lorawan.NewChannelReq},
				{CID: lorawan.LinkADRReq},
			},
			Expected: []storage.MACCommandBlock{
				{CID: lorawan.NewChannelReq},
			},
		},
		{
			Name: "LinkADRReq + NewChannelReq",
			MACCommands: []storage.MACCommandBlock{
				{CID: lorawan.LinkADRReq},
				{CID: lorawan.NewChannelReq},
			},
			Expected: []storage.MACCommandBlock{
				{CID: lorawan.NewChannelReq},
			},
		},
	}

	for _, tst := range tests {
		t.Run(tst.Name, func(t *testing.T) {
			assert := require.New(t)

			out := filterIncompatibleMACCommands(tst.MACCommands)
			assert.Equal(tst.Expected, out)
		})
	}
}

func TestSetTXParameters(t *testing.T) {
	tests := []struct {
		Name string

		Band                   loraband.Name
		UplinkDwellTime400ms   bool
		DownlinkDwellTime400ms bool
		UplinkMaxEIRP          float32

		DeviceProfile storage.DeviceProfile
		DeviceSession storage.DeviceSession

		ExpectedMACCommands []storage.MACCommandBlock
	}{
		{
			Name:                   "Band does not implement TXParamSetup",
			Band:                   loraband.EU868,
			UplinkDwellTime400ms:   true,
			DownlinkDwellTime400ms: true,
			UplinkMaxEIRP:          16,
			DeviceProfile: storage.DeviceProfile{
				MaxEIRP: 16,
			},
			DeviceSession: storage.DeviceSession{
				UplinkDwellTime400ms:   false,
				DownlinkDwellTime400ms: false,
				UplinkMaxEIRPIndex:     0,
			},
		},
		{
			Name:                   "Band does implement TXParamSetup",
			Band:                   loraband.AS923,
			UplinkDwellTime400ms:   true,
			DownlinkDwellTime400ms: true,
			UplinkMaxEIRP:          16,
			DeviceProfile: storage.DeviceProfile{
				MaxEIRP: 16,
			},
			DeviceSession: storage.DeviceSession{
				UplinkDwellTime400ms:   false,
				DownlinkDwellTime400ms: false,
				UplinkMaxEIRPIndex:     0,
			},
			ExpectedMACCommands: []storage.MACCommandBlock{
				{
					CID: lorawan.TXParamSetupReq,
					MACCommands: []lorawan.MACCommand{
						{
							CID: lorawan.TXParamSetupReq,
							Payload: &lorawan.TXParamSetupReqPayload{
								UplinkDwellTime:   lorawan.DwellTime400ms,
								DownlinkDwelltime: lorawan.DwellTime400ms,
								MaxEIRP:           5,
							},
						},
					},
				},
			},
		},
		{
			Name:                   "Band does implement TXParamSetup - MaxEIRP limited by device-profile",
			Band:                   loraband.AS923,
			UplinkDwellTime400ms:   true,
			DownlinkDwellTime400ms: true,
			UplinkMaxEIRP:          16,
			DeviceProfile: storage.DeviceProfile{
				MaxEIRP: 10,
			},
			DeviceSession: storage.DeviceSession{
				UplinkDwellTime400ms:   false,
				DownlinkDwellTime400ms: false,
				UplinkMaxEIRPIndex:     0,
			},
			ExpectedMACCommands: []storage.MACCommandBlock{
				{
					CID: lorawan.TXParamSetupReq,
					MACCommands: []lorawan.MACCommand{
						{
							CID: lorawan.TXParamSetupReq,
							Payload: &lorawan.TXParamSetupReqPayload{
								UplinkDwellTime:   lorawan.DwellTime400ms,
								DownlinkDwelltime: lorawan.DwellTime400ms,
								MaxEIRP:           1,
							},
						},
					},
				},
			},
		},
		{
			Name:                   "Band does implement TXParamSetup - MaxEIRP limited by network settings",
			Band:                   loraband.AS923,
			UplinkDwellTime400ms:   true,
			DownlinkDwellTime400ms: true,
			UplinkMaxEIRP:          16,
			DeviceProfile: storage.DeviceProfile{
				MaxEIRP: 30,
			},
			DeviceSession: storage.DeviceSession{
				UplinkDwellTime400ms:   false,
				DownlinkDwellTime400ms: false,
				UplinkMaxEIRPIndex:     0,
			},
			ExpectedMACCommands: []storage.MACCommandBlock{
				{
					CID: lorawan.TXParamSetupReq,
					MACCommands: []lorawan.MACCommand{
						{
							CID: lorawan.TXParamSetupReq,
							Payload: &lorawan.TXParamSetupReqPayload{
								UplinkDwellTime:   lorawan.DwellTime400ms,
								DownlinkDwelltime: lorawan.DwellTime400ms,
								MaxEIRP:           5,
							},
						},
					},
				},
			},
		},
	}

	for _, tst := range tests {
		t.Run(tst.Name, func(t *testing.T) {
			assert := require.New(t)

			var c config.Config
			c.NetworkServer.Band.Name = tst.Band
			c.NetworkServer.Band.UplinkDwellTime400ms = tst.UplinkDwellTime400ms
			c.NetworkServer.Band.DownlinkDwellTime400ms = tst.DownlinkDwellTime400ms
			c.NetworkServer.Band.UplinkMaxEIRP = tst.UplinkMaxEIRP

			assert.NoError(band.Setup(c))
			assert.NoError(Setup(c))

			ctx := dataContext{
				DeviceSession: tst.DeviceSession,
				DeviceProfile: tst.DeviceProfile,
			}

			assert.NoError(setTXParameters(&ctx))
			assert.Equal(tst.ExpectedMACCommands, ctx.MACCommands)
		})
	}
}

func TestPreferRX2DR(t *testing.T) {
	assert := require.New(t)
	conf := test.GetConfig()
	assert.NoError(Setup(conf))

	tests := []struct {
		Name               string
		DeviceSession      storage.DeviceSession
		RX2PreferOnRX1DRLt int
		RX2Prefered        bool
	}{
		{
			Name:               "Uplink DR0 - RX1",
			RX2PreferOnRX1DRLt: 0,
			RX2Prefered:        false,
			DeviceSession: storage.DeviceSession{
				RX2DR:        0,
				RX2Frequency: 869525000,
				DR:           0,
			},
		},
		{
			Name:               "Uplink DR0, RX2 prefered on DR < 3 - RX2",
			RX2PreferOnRX1DRLt: 3,
			RX2Prefered:        true,
			DeviceSession: storage.DeviceSession{
				RX2DR:        0,
				RX2Frequency: 869525000,
				DR:           0,
			},
		},
		{
			Name:               "Uplink DR0, RX2 prefered on DR < 3, device reconfig pending - RX1",
			RX2PreferOnRX1DRLt: 3,
			RX2Prefered:        false,
			DeviceSession: storage.DeviceSession{
				RX2DR:        1,
				RX2Frequency: 869525000,
				DR:           0,
			},
		},
	}

	for _, tst := range tests {
		t.Run(tst.Name, func(t *testing.T) {
			assert := require.New(t)

			rx2PreferOnRX1DRLt = tst.RX2PreferOnRX1DRLt

			ctx := dataContext{
				DeviceSession: tst.DeviceSession,
				RXPacket:      &models.RXPacket{},
			}

			prefered, err := preferRX2DR(&ctx)
			assert.NoError(err)

			assert.Equal(tst.RX2Prefered, prefered)
		})
	}
}

func TestPreferRX2LinkBudget(t *testing.T) {
	assert := require.New(t)
	conf := test.GetConfig()
	conf.NetworkServer.NetworkSettings.RX2PreferOnLinkBudget = true
	assert.NoError(Setup(conf))

	tests := []struct {
		Name                  string
		RX2PreferOnLinkBudget bool
		DeviceSession         storage.DeviceSession
		RX2Prefered           bool
		DownlinkTXPower       int
	}{
		{
			Name:            "Uplink DR0 - RX2",
			RX2Prefered:     true,
			DownlinkTXPower: -1,
			DeviceSession: storage.DeviceSession{
				RX2DR:        0,
				RX2Frequency: 869525000,
				DR:           0,
			},
		},
		{
			Name:            "Uplink DR5 - RX2",
			RX2Prefered:     true,
			DownlinkTXPower: -1,
			DeviceSession: storage.DeviceSession{
				RX2DR:        0,
				RX2Frequency: 869525000,
				DR:           5,
			},
		},
		{
			Name:            "Uplink DR5 - custom tx power - RX1",
			RX2Prefered:     true,
			DownlinkTXPower: 14,
			DeviceSession: storage.DeviceSession{
				RX2DR:        0,
				RX2Frequency: 869525000,
				DR:           5,
			},
		},
	}

	for _, tst := range tests {
		t.Run(tst.Name, func(t *testing.T) {
			assert := require.New(t)

			downlinkTXPower = tst.DownlinkTXPower

			ctx := dataContext{
				DeviceSession: tst.DeviceSession,
				RXPacket: &models.RXPacket{
					TXInfo: &gw.UplinkTXInfo{
						Frequency: 868100000,
					},
				},
			}

			prefered, err := preferRX2LinkBudget(&ctx)
			assert.NoError(err)

			assert.Equal(tst.RX2Prefered, prefered)
		})
	}
}

func TestSetDataTXInfo(t *testing.T) {
	assert := require.New(t)
	conf := test.GetConfig()
	assert.NoError(Setup(conf))

	tests := []struct {
		Name                   string
		RXWindow               int
		RX2PreferOnLinkBudget  bool
		RX2PreferOnRX1DRLt     int
		DeviceSession          storage.DeviceSession
		UplinkFrequency        int
		DownlinkTXPower        int
		ExpectedDownlinkTXInfo []*gw.DownlinkTXInfo
	}{
		{
			Name:     "RX1 only",
			RXWindow: 1,
			DeviceSession: storage.DeviceSession{
				RX2DR:        1,
				RX2Frequency: 869525000,
				DR:           3,
			},
			UplinkFrequency: 868100000,
			DownlinkTXPower: -1,
			ExpectedDownlinkTXInfo: []*gw.DownlinkTXInfo{
				{
					Frequency:  868100000,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       9,
							CodeRate:              "4/5",
							PolarizationInversion: true,
						},
					},
					Timing: gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: &duration.Duration{
								Seconds: 1,
							},
						},
					},
				},
			},
		},
		{
			Name:     "RX2 only",
			RXWindow: 2,
			DeviceSession: storage.DeviceSession{
				RX2DR:        1,
				RX2Frequency: 869525000,
				DR:           3,
			},
			UplinkFrequency: 868100000,
			DownlinkTXPower: -1,
			ExpectedDownlinkTXInfo: []*gw.DownlinkTXInfo{
				{
					Frequency:  869525000,
					Power:      27,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       11,
							CodeRate:              "4/5",
							PolarizationInversion: true,
						},
					},
					Timing: gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: &duration.Duration{
								Seconds: 2,
							},
						},
					},
				},
			},
		},
		{
			Name:     "Prefer RX2 DR - RX1",
			RXWindow: 0,
			DeviceSession: storage.DeviceSession{
				RX2DR:        1,
				RX2Frequency: 869525000,
				DR:           3,
			},
			UplinkFrequency:    868100000,
			RX2PreferOnRX1DRLt: 3,
			DownlinkTXPower:    -1,
			ExpectedDownlinkTXInfo: []*gw.DownlinkTXInfo{
				{
					Frequency:  868100000,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       9,
							CodeRate:              "4/5",
							PolarizationInversion: true,
						},
					},
					Timing: gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: &duration.Duration{
								Seconds: 1,
							},
						},
					},
				},
				{
					Frequency:  869525000,
					Power:      27,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       11,
							CodeRate:              "4/5",
							PolarizationInversion: true,
						},
					},
					Timing: gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: &duration.Duration{
								Seconds: 2,
							},
						},
					},
				},
			},
		},
		{
			Name:     "Prefer RX2 DR - RX2",
			RXWindow: 0,
			DeviceSession: storage.DeviceSession{
				RX2DR:        1,
				RX2Frequency: 869525000,
				DR:           3,
			},
			UplinkFrequency:    868100000,
			RX2PreferOnRX1DRLt: 4,
			DownlinkTXPower:    -1,
			ExpectedDownlinkTXInfo: []*gw.DownlinkTXInfo{
				{
					Frequency:  869525000,
					Power:      27,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       11,
							CodeRate:              "4/5",
							PolarizationInversion: true,
						},
					},
					Timing: gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: &duration.Duration{
								Seconds: 2,
							},
						},
					},
				},
				{
					Frequency:  868100000,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       9,
							CodeRate:              "4/5",
							PolarizationInversion: true,
						},
					},
					Timing: gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: &duration.Duration{
								Seconds: 1,
							},
						},
					},
				},
			},
		},
		{
			Name:     "Prefer RX2 link-budget - RX1",
			RXWindow: 0,
			DeviceSession: storage.DeviceSession{
				RX2DR:        1,
				RX2Frequency: 869525000,
				DR:           0,
			},
			UplinkFrequency:       868100000,
			RX2PreferOnLinkBudget: true,
			DownlinkTXPower:       14,
			ExpectedDownlinkTXInfo: []*gw.DownlinkTXInfo{
				{
					Frequency:  868100000,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       12,
							CodeRate:              "4/5",
							PolarizationInversion: true,
						},
					},
					Timing: gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: &duration.Duration{
								Seconds: 1,
							},
						},
					},
				},
				{
					Frequency:  869525000,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       11,
							CodeRate:              "4/5",
							PolarizationInversion: true,
						},
					},
					Timing: gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: &duration.Duration{
								Seconds: 2,
							},
						},
					},
				},
			},
		},
		{
			Name:     "Prefer RX2 link-budget - RX2",
			RXWindow: 0,
			DeviceSession: storage.DeviceSession{
				RX2DR:        1,
				RX2Frequency: 869525000,
				DR:           2,
			},
			UplinkFrequency:       868100000,
			RX2PreferOnLinkBudget: true,
			DownlinkTXPower:       14,
			ExpectedDownlinkTXInfo: []*gw.DownlinkTXInfo{
				{
					Frequency:  869525000,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       11,
							CodeRate:              "4/5",
							PolarizationInversion: true,
						},
					},
					Timing: gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: &duration.Duration{
								Seconds: 2,
							},
						},
					},
				},
				{
					Frequency:  868100000,
					Power:      14,
					Modulation: common.Modulation_LORA,
					ModulationInfo: &gw.DownlinkTXInfo_LoraModulationInfo{
						LoraModulationInfo: &gw.LoRaModulationInfo{
							Bandwidth:             125,
							SpreadingFactor:       10,
							CodeRate:              "4/5",
							PolarizationInversion: true,
						},
					},
					Timing: gw.DownlinkTiming_DELAY,
					TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
						DelayTimingInfo: &gw.DelayTimingInfo{
							Delay: &duration.Duration{
								Seconds: 1,
							},
						},
					},
				},
			},
		},
	}

	for _, tst := range tests {
		t.Run(tst.Name, func(t *testing.T) {
			assert := require.New(t)

			rx2PreferOnRX1DRLt = tst.RX2PreferOnRX1DRLt
			rx2PreferOnLinkBudget = tst.RX2PreferOnLinkBudget
			downlinkTXPower = tst.DownlinkTXPower

			rxWindow = tst.RXWindow
			rx2Frequency = tst.DeviceSession.RX2Frequency
			rx2DR = int(tst.DeviceSession.RX2DR)

			ctx := dataContext{
				DeviceSession:       tst.DeviceSession,
				DeviceGatewayRXInfo: []storage.DeviceGatewayRXInfo{{}},
				RXPacket: &models.RXPacket{
					TXInfo: &gw.UplinkTXInfo{
						Frequency: uint32(tst.UplinkFrequency),
					},
				},
			}

			assert.NoError(setDataTXInfo(&ctx))

			var txInfo []*gw.DownlinkTXInfo
			for i := range ctx.DownlinkFrameItems {
				txInfo = append(txInfo, ctx.DownlinkFrameItems[i].DownlinkFrameItem.TxInfo)
			}

			assert.EqualValues(tst.ExpectedDownlinkTXInfo, txInfo)
		})
	}
}

func TestSetPHYPayloads(t *testing.T) {
	tests := []struct {
		name                       string
		downlinkFrameItems         []downlinkFrameItem
		deviceSession              storage.DeviceSession
		deviceQueueItem            *storage.DeviceQueueItem
		macCommandBlocks           []storage.MACCommandBlock
		expectedDownlinkFrameItems []*gw.DownlinkFrameItem
	}{
		{
			name: "frmpayload for rx1 and rx2",
			downlinkFrameItems: []downlinkFrameItem{
				{
					RemainingPayloadSize: 50,
				},
				{
					RemainingPayloadSize: 50,
				},
			},
			deviceSession: storage.DeviceSession{
				MACVersion: "1.0.3",
				NFCntDown:  10,
			},
			deviceQueueItem: &storage.DeviceQueueItem{
				FRMPayload: []byte{1, 2, 3},
				FCnt:       10,
				FPort:      20,
				Confirmed:  true,
			},
			expectedDownlinkFrameItems: []*gw.DownlinkFrameItem{
				{
					PhyPayload: []byte{0xa0, 0x0, 0x0, 0x0, 0x0, 0x80, 0xa, 0x0, 0x14, 0x1, 0x2, 0x3, 0x30, 0xb5, 0x95, 0x6f},
				},
				{
					PhyPayload: []byte{0xa0, 0x0, 0x0, 0x0, 0x0, 0x80, 0xa, 0x0, 0x14, 0x1, 0x2, 0x3, 0x30, 0xb5, 0x95, 0x6f},
				},
			},
		},
		{
			name: "frmpayload for rx1, empty frame for rx2",
			downlinkFrameItems: []downlinkFrameItem{
				{
					RemainingPayloadSize: 50,
				},
				{
					RemainingPayloadSize: 2,
				},
			},
			deviceSession: storage.DeviceSession{
				MACVersion: "1.0.3",
				NFCntDown:  10,
			},
			deviceQueueItem: &storage.DeviceQueueItem{
				FRMPayload: []byte{1, 2, 3},
				FCnt:       10,
				FPort:      20,
				Confirmed:  true,
			},
			expectedDownlinkFrameItems: []*gw.DownlinkFrameItem{
				{
					PhyPayload: []byte{0xa0, 0x0, 0x0, 0x0, 0x0, 0x80, 0xa, 0x0, 0x14, 0x1, 0x2, 0x3, 0x30, 0xb5, 0x95, 0x6f},
				},
				{
					PhyPayload: []byte{0x60, 0x0, 0x0, 0x0, 0x0, 0x90, 0xa, 0x0, 0xaf, 0x79, 0x8a, 0x46},
				},
			},
		},
		{
			name: "empty frame for rx1, frmpayload for rx2",
			downlinkFrameItems: []downlinkFrameItem{
				{
					RemainingPayloadSize: 2,
				},
				{
					RemainingPayloadSize: 50,
				},
			},
			deviceSession: storage.DeviceSession{
				MACVersion: "1.0.3",
				NFCntDown:  10,
			},
			deviceQueueItem: &storage.DeviceQueueItem{
				FRMPayload: []byte{1, 2, 3},
				FCnt:       10,
				FPort:      20,
				Confirmed:  true,
			},
			expectedDownlinkFrameItems: []*gw.DownlinkFrameItem{
				{
					PhyPayload: []byte{0x60, 0x0, 0x0, 0x0, 0x0, 0x90, 0xa, 0x0, 0xaf, 0x79, 0x8a, 0x46},
				},
				{
					PhyPayload: []byte{0xa0, 0x0, 0x0, 0x0, 0x0, 0x80, 0xa, 0x0, 0x14, 0x1, 0x2, 0x3, 0x30, 0xb5, 0x95, 0x6f},
				},
			},
		},
		{
			name: "mac-commands + frmpayload for rx1, mac-commands for rx2",
			downlinkFrameItems: []downlinkFrameItem{
				{
					RemainingPayloadSize: 10,
				},
				{
					RemainingPayloadSize: 2,
				},
			},
			deviceSession: storage.DeviceSession{
				MACVersion: "1.0.3",
				NFCntDown:  10,
			},
			deviceQueueItem: &storage.DeviceQueueItem{
				FRMPayload: []byte{1, 2, 3},
				FCnt:       10,
				FPort:      20,
				Confirmed:  true,
			},
			macCommandBlocks: []storage.MACCommandBlock{
				{
					CID: lorawan.DevStatusReq,
					MACCommands: []lorawan.MACCommand{
						{
							CID: lorawan.DevStatusReq,
						},
					},
				},
			},
			expectedDownlinkFrameItems: []*gw.DownlinkFrameItem{
				{
					PhyPayload: []byte{0xa0, 0x0, 0x0, 0x0, 0x0, 0x81, 0xa, 0x0, 0x6, 0x14, 0x1, 0x2, 0x3, 0x93, 0xf2, 0x86, 0x2e},
				},
				{
					PhyPayload: []byte{0x60, 0x0, 0x0, 0x0, 0x0, 0x91, 0xa, 0x0, 0x6, 0x7f, 0xf3, 0x22, 0xff},
				},
			},
		},
		{
			name: "mac-commands for rx1 and rx2",
			downlinkFrameItems: []downlinkFrameItem{
				{
					RemainingPayloadSize: 2,
				},
				{
					RemainingPayloadSize: 2,
				},
			},
			deviceSession: storage.DeviceSession{
				MACVersion: "1.0.3",
				NFCntDown:  10,
			},
			deviceQueueItem: &storage.DeviceQueueItem{
				FRMPayload: []byte{1, 2, 3},
				FCnt:       10,
				FPort:      20,
				Confirmed:  true,
			},
			macCommandBlocks: []storage.MACCommandBlock{
				{
					CID: lorawan.DevStatusReq,
					MACCommands: []lorawan.MACCommand{
						{
							CID: lorawan.DevStatusReq,
						},
					},
				},
			},
			expectedDownlinkFrameItems: []*gw.DownlinkFrameItem{
				{
					PhyPayload: []byte{0x60, 0x0, 0x0, 0x0, 0x0, 0x91, 0xa, 0x0, 0x6, 0x7f, 0xf3, 0x22, 0xff},
				},
				{
					PhyPayload: []byte{0x60, 0x0, 0x0, 0x0, 0x0, 0x91, 0xa, 0x0, 0x6, 0x7f, 0xf3, 0x22, 0xff},
				},
			},
		},
	}

	for _, tst := range tests {
		t.Run(tst.name, func(t *testing.T) {
			assert := require.New(t)

			ctx := dataContext{
				DownlinkFrameItems: tst.downlinkFrameItems,
				DeviceSession:      tst.deviceSession,
				DeviceQueueItem:    tst.deviceQueueItem,
				MACCommands:        tst.macCommandBlocks,
			}

			assert.NoError(setPHYPayloads(&ctx))
			assert.Equal(len(tst.expectedDownlinkFrameItems), len(ctx.DownlinkFrame.Items))

			for i := range tst.expectedDownlinkFrameItems {
				assert.Equal(tst.expectedDownlinkFrameItems[i], ctx.DownlinkFrame.Items[i])
			}
		})
	}
}
