package data

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/brocaar/loraserver/internal/backend/applicationserver"
	"github.com/brocaar/loraserver/internal/band"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/loraserver/internal/test"
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
	assert.NoError(storage.CreateServiceProfile(storage.DB(), &sp))

	dp := storage.DeviceProfile{}
	assert.NoError(storage.CreateDeviceProfile(storage.DB(), &dp))

	rp := storage.RoutingProfile{}
	assert.NoError(storage.CreateRoutingProfile(storage.DB(), &rp))

	ts.Device = storage.Device{
		DevEUI:           lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
		ServiceProfileID: sp.ID,
		DeviceProfileID:  dp.ID,
		RoutingProfileID: rp.ID,
	}
	assert.NoError(storage.CreateDevice(storage.DB(), &ts.Device))
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
				DownlinkFrames: []downlinkFrame{
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
				DownlinkFrames: []downlinkFrame{
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
				DownlinkFrames: []downlinkFrame{
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
				DownlinkFrames: []downlinkFrame{
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
				DownlinkFrames: []downlinkFrame{
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
				DownlinkFrames: []downlinkFrame{
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

			assert.NoError(storage.FlushDeviceQueueForDevEUI(storage.DB(), ts.Device.DevEUI))
			for i := range tst.DeviceQueueItems {
				assert.NoError(storage.CreateDeviceQueueItem(storage.DB(), &tst.DeviceQueueItems[i]))
			}

			assert.NoError(getNextDeviceQueueItem(&tst.DataContext))
			assert.Equal(tst.ExpectedDataContext, tst.DataContext)

			if tst.ExpectedNextDeviceQueueItem != nil {
				qi, err := storage.GetNextDeviceQueueItemForDevEUI(storage.DB(), ts.Device.DevEUI)
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
				DownlinkFrames: []downlinkFrame{
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
				DownlinkFrames: []downlinkFrame{
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
				DownlinkFrames: []downlinkFrame{
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
				DownlinkFrames: []downlinkFrame{
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
				DownlinkFrames: []downlinkFrame{
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
				DownlinkFrames: []downlinkFrame{
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
				DownlinkFrames: []downlinkFrame{
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
				DownlinkFrames: []downlinkFrame{
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
				DownlinkFrames: []downlinkFrame{
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
				DownlinkFrames: []downlinkFrame{
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
				DownlinkFrames: []downlinkFrame{
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
				DownlinkFrames: []downlinkFrame{
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
				DownlinkFrames: []downlinkFrame{
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

			test.MustFlushRedis(storage.RedisPool())

			if tst.BeforeFunc != nil {
				assert.NoError(tst.BeforeFunc())
			}

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
