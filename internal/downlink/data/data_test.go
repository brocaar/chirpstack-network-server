package data

import (
	"context"
	"testing"

	"github.com/golang/protobuf/ptypes/duration"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/brocaar/chirpstack-api/go/v3/common"
	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/brocaar/chirpstack-network-server/internal/backend/applicationserver"
	"github.com/brocaar/chirpstack-network-server/internal/band"
	"github.com/brocaar/chirpstack-network-server/internal/config"
	"github.com/brocaar/chirpstack-network-server/internal/models"
	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/brocaar/chirpstack-network-server/internal/test"
	"github.com/brocaar/lorawan"
	loraband "github.com/brocaar/lorawan/band"
)

type GetNextDeviceQueueItemTestSuite struct {
	suite.Suite

	tx *storage.TxLogger

	ASClient *test.ApplicationClient
	Device   storage.Device
}

func (ts *GetNextDeviceQueueItemTestSuite) SetupSuite() {
	assert := require.New(ts.T())
	conf := test.GetConfig()
	assert.NoError(storage.Setup(conf))
	test.MustResetDB(storage.DB().DB)

	ts.ASClient = test.NewApplicationClient()
	applicationserver.SetPool(test.NewApplicationServerPool(ts.ASClient))

	sp := storage.ServiceProfile{}
	assert.NoError(storage.CreateServiceProfile(context.Background(), storage.DB(), &sp))

	dp := storage.DeviceProfile{}
	assert.NoError(storage.CreateDeviceProfile(context.Background(), storage.DB(), &dp))

	rp := storage.RoutingProfile{}
	assert.NoError(storage.CreateRoutingProfile(context.Background(), storage.DB(), &rp))

	ts.Device = storage.Device{
		DevEUI:           lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
		ServiceProfileID: sp.ID,
		DeviceProfileID:  dp.ID,
		RoutingProfileID: rp.ID,
	}
	assert.NoError(storage.CreateDevice(context.Background(), storage.DB(), &ts.Device))
}

func (ts *GetNextDeviceQueueItemTestSuite) SetupTest() {
	assert := require.New(ts.T())
	var err error
	ts.tx, err = storage.DB().Beginx()
	assert.NoError(err)
}

func (ts *GetNextDeviceQueueItemTestSuite) TestGetNextDeviceQueueItem() {
	tests := []struct {
		Name                        string
		DeviceQueueItems            []storage.DeviceQueueItem
		DataContext                 dataContext
		ExpectedDataContext         dataContext
		ExpectedNextDeviceQueueItem *storage.DeviceQueueItem
	}{
		{
			Name: "remove all queue items because of frame-counter gap",
			DeviceQueueItems: []storage.DeviceQueueItem{
				{
					DevEUI:     ts.Device.DevEUI,
					FRMPayload: []byte{1, 2, 3, 4},
					FCnt:       10,
					FPort:      1,
				},
				{
					DevEUI:     ts.Device.DevEUI,
					FRMPayload: []byte{4, 5, 6, 7},
					Confirmed:  true,
					FCnt:       11,
					FPort:      1,
				},
			},
			DataContext: dataContext{
				DeviceSession: storage.DeviceSession{
					RoutingProfileID: ts.Device.RoutingProfileID,
					DevEUI:           ts.Device.DevEUI,
					NFCntDown:        12,
				},
				DownlinkFrameItems: []downlinkFrameItem{
					{
						RemainingPayloadSize: 242,
					},
				},
			},
			ExpectedDataContext: dataContext{
				DeviceSession: storage.DeviceSession{
					RoutingProfileID: ts.Device.RoutingProfileID,
					DevEUI:           ts.Device.DevEUI,
					NFCntDown:        12,
				},
				DownlinkFrameItems: []downlinkFrameItem{
					{
						RemainingPayloadSize: 242,
					},
				},
			},
		},
		{
			Name: "first queue item (unconfirmed)",
			DeviceQueueItems: []storage.DeviceQueueItem{
				{
					DevEUI:     ts.Device.DevEUI,
					FRMPayload: []byte{1, 2, 3, 4},
					FCnt:       10,
					FPort:      1,
				},
				{
					DevEUI:     ts.Device.DevEUI,
					FRMPayload: []byte{4, 5, 6, 7},
					Confirmed:  true,
					FCnt:       11,
					FPort:      1,
				},
			},
			DataContext: dataContext{
				DeviceSession: storage.DeviceSession{
					RoutingProfileID: ts.Device.RoutingProfileID,
					DevEUI:           ts.Device.DevEUI,
					NFCntDown:        10,
				},
				DownlinkFrameItems: []downlinkFrameItem{
					{
						RemainingPayloadSize: 242,
					},
				},
			},
			ExpectedDataContext: dataContext{
				DeviceSession: storage.DeviceSession{
					RoutingProfileID: ts.Device.RoutingProfileID,
					DevEUI:           ts.Device.DevEUI,
					NFCntDown:        10,
				},
				Data:     []byte{1, 2, 3, 4},
				FPort:    1,
				MoreData: true,
				DownlinkFrameItems: []downlinkFrameItem{
					{
						RemainingPayloadSize: 242 - 4,
					},
				},
			},
			// the seconds item should be returned as the first item
			// has been popped from the queue
			ExpectedNextDeviceQueueItem: &storage.DeviceQueueItem{
				DevEUI:     ts.Device.DevEUI,
				FRMPayload: []byte{4, 5, 6, 7},
				FPort:      1,
				FCnt:       11,
				Confirmed:  true,
			},
		},
		{
			Name: "second queue item (confirmed)",
			DeviceQueueItems: []storage.DeviceQueueItem{
				{
					DevEUI:     ts.Device.DevEUI,
					FRMPayload: []byte{1, 2, 3, 4},
					FCnt:       10,
					FPort:      1,
				},
				{
					DevEUI:     ts.Device.DevEUI,
					FRMPayload: []byte{4, 5, 6, 7},
					Confirmed:  true,
					FCnt:       11,
					FPort:      1,
				},
			},
			DataContext: dataContext{
				DeviceSession: storage.DeviceSession{
					RoutingProfileID: ts.Device.RoutingProfileID,
					DevEUI:           ts.Device.DevEUI,
					NFCntDown:        11, // so the first one is skipped
				},
				DownlinkFrameItems: []downlinkFrameItem{
					{
						RemainingPayloadSize: 242,
					},
				},
			},
			ExpectedDataContext: dataContext{
				DeviceSession: storage.DeviceSession{
					RoutingProfileID: ts.Device.RoutingProfileID,
					DevEUI:           ts.Device.DevEUI,
					NFCntDown:        11,
					ConfFCnt:         11,
				},
				Data:      []byte{4, 5, 6, 7},
				FPort:     1,
				Confirmed: true,
				DownlinkFrameItems: []downlinkFrameItem{
					{
						RemainingPayloadSize: 242 - 4,
					},
				},
			},
			// the seconds item should be returned as the first item
			// has been popped from the queue
			ExpectedNextDeviceQueueItem: &storage.DeviceQueueItem{
				DevEUI:     ts.Device.DevEUI,
				FRMPayload: []byte{4, 5, 6, 7},
				FPort:      1,
				FCnt:       11,
				Confirmed:  true,
				IsPending:  true,
			},
		},
	}

	for _, tst := range tests {
		ts.T().Run(tst.Name, func(t *testing.T) {
			assert := require.New(t)
			ctx := context.Background()
			tst.DataContext.ctx = ctx

			assert.NoError(storage.FlushDeviceQueueForDevEUI(ctx, storage.DB(), ts.Device.DevEUI))
			for i := range tst.DeviceQueueItems {
				assert.NoError(storage.CreateDeviceQueueItem(context.Background(), storage.DB(), &tst.DeviceQueueItems[i]))
			}

			assert.NoError(getNextDeviceQueueItem(&tst.DataContext))

			tst.DataContext.ctx = nil
			assert.Equal(tst.ExpectedDataContext, tst.DataContext)

			if tst.ExpectedNextDeviceQueueItem != nil {
				qi, err := storage.GetNextDeviceQueueItemForDevEUI(context.Background(), storage.DB(), ts.Device.DevEUI)
				assert.NoError(err)
				assert.Equal(tst.ExpectedNextDeviceQueueItem.FRMPayload, qi.FRMPayload)
				assert.Equal(tst.ExpectedNextDeviceQueueItem.FPort, qi.FPort)
				assert.Equal(tst.ExpectedNextDeviceQueueItem.FCnt, qi.FCnt)
				assert.Equal(tst.ExpectedNextDeviceQueueItem.IsPending, qi.IsPending)
				assert.Equal(tst.ExpectedNextDeviceQueueItem.Confirmed, qi.Confirmed)
				if tst.ExpectedNextDeviceQueueItem.IsPending {
					assert.NotNil(qi.TimeoutAfter)
				}
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
