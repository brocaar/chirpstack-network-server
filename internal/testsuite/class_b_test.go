package testsuite

import (
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/downlink"
	"github.com/brocaar/loraserver/internal/gps"
	"github.com/brocaar/loraserver/internal/helpers"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/loraserver/internal/test"
	"github.com/brocaar/loraserver/internal/uplink"
	"github.com/brocaar/lorawan"
)

type ClassBTestSuite struct {
	suite.Suite
	test.DatabaseTestSuiteBase

	Device  storage.Device
	Gateway storage.Gateway
}

func (ts *ClassBTestSuite) SetupSuite() {
	ts.DatabaseTestSuiteBase.SetupSuite()

	assert := require.New(ts.T())

	asClient := test.NewApplicationClient()
	config.C.ApplicationServer.Pool = test.NewApplicationServerPool(asClient)
	config.C.NetworkController.Client = test.NewNetworkControllerClient()
	config.C.NetworkServer.Gateway.Backend.Backend = test.NewGatewayBackend()
	config.C.NetworkServer.NetworkSettings.ClassB.PingSlotDR = 2
	config.C.NetworkServer.NetworkSettings.ClassB.PingSlotFrequency = 868300000

	ts.Gateway = storage.Gateway{
		MAC: [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
		Location: storage.GPSPoint{
			Latitude:  1.1234,
			Longitude: 1.1235,
		},
		Altitude: 10.5,
	}
	assert.NoError(storage.CreateGateway(ts.DB(), &ts.Gateway))

	// service-profile
	sp := storage.ServiceProfile{
		AddGWMetadata: true,
	}
	assert.NoError(storage.CreateServiceProfile(ts.DB(), &sp))

	// device-profile
	dp := storage.DeviceProfile{
		SupportsClassB: true,
	}
	assert.NoError(storage.CreateDeviceProfile(ts.DB(), &dp))

	// routing-profile
	rp := storage.RoutingProfile{}
	assert.NoError(storage.CreateRoutingProfile(ts.DB(), &rp))

	// device
	ts.Device = storage.Device{
		ServiceProfileID: sp.ID,
		DeviceProfileID:  dp.ID,
		RoutingProfileID: rp.ID,
		DevEUI:           lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
	}
	assert.NoError(storage.CreateDevice(ts.DB(), &ts.Device))
}

func (ts *ClassBTestSuite) SetupTest() {
	ts.DatabaseTestSuiteBase.SetupTest()

	assert := require.New(ts.T())
	assert.NoError(storage.FlushDeviceQueueForDevEUI(ts.DB(), ts.Device.DevEUI))
}

func (ts *ClassBTestSuite) TestUplink() {
	assert := require.New(ts.T())

	queueItems := []storage.DeviceQueueItem{
		{
			DevEUI:     ts.Device.DevEUI,
			FRMPayload: []byte{1, 2, 3, 4},
			FPort:      1,
			FCnt:       1,
		},
		{
			DevEUI:     ts.Device.DevEUI,
			FRMPayload: []byte{1, 2, 3, 4},
			FPort:      1,
			FCnt:       2,
		},
		{
			DevEUI:     ts.Device.DevEUI,
			FRMPayload: []byte{1, 2, 3, 4},
			FPort:      1,
			FCnt:       3,
		},
	}
	for i := range queueItems {
		assert.NoError(storage.CreateDeviceQueueItem(ts.DB(), &queueItems[i]))
	}

	// device-session
	ds := storage.DeviceSession{
		DeviceProfileID:  ts.Device.DeviceProfileID,
		ServiceProfileID: ts.Device.ServiceProfileID,
		RoutingProfileID: ts.Device.RoutingProfileID,
		DevEUI:           ts.Device.DevEUI,
		JoinEUI:          lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1},

		DevAddr:               lorawan.DevAddr{1, 2, 3, 4},
		FNwkSIntKey:           [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SNwkSIntKey:           [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		NwkSEncKey:            [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		FCntUp:                8,
		NFCntDown:             5,
		EnabledUplinkChannels: []int{0, 1, 2},
		PingSlotNb:            1,
		RX2Frequency:          869525000,
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	c0, err := config.C.NetworkServer.Band.Band.GetUplinkChannel(0)
	assert.NoError(err)

	rxInfo := gw.UplinkRXInfo{
		GatewayId: ts.Gateway.MAC[:],
		LoraSnr:   7,
	}
	rxInfo.Time, _ = ptypes.TimestampProto(now)
	rxInfo.TimeSinceGpsEpoch = ptypes.DurationProto(10 * time.Second)

	txInfo := gw.UplinkTXInfo{
		Frequency: uint32(c0.Frequency),
	}
	assert.NoError(helpers.SetUplinkTXInfoDataRate(&txInfo, 0, config.C.NetworkServer.Band.Band))

	testTable := []struct {
		BeforeFunc           func(ds *storage.DeviceSession)
		Name                 string
		DeviceSession        storage.DeviceSession
		PHYPayload           lorawan.PHYPayload
		ExpectedBeaconLocked bool
	}{
		{
			Name:          "trigger beacon locked",
			DeviceSession: ds,
			PHYPayload: lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					Major: lorawan.LoRaWANR1,
					MType: lorawan.UnconfirmedDataUp,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ds.DevAddr,
						FCnt:    ds.FCntUp,
						FCtrl: lorawan.FCtrl{
							ClassB: true,
						},
					},
				},
			},
			ExpectedBeaconLocked: true,
		},
		{
			BeforeFunc: func(ds *storage.DeviceSession) {
				ds.BeaconLocked = true
			},
			Name:          "trigger beacon unlocked",
			DeviceSession: ds,
			PHYPayload: lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					Major: lorawan.LoRaWANR1,
					MType: lorawan.UnconfirmedDataUp,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ds.DevAddr,
						FCnt:    ds.FCntUp,
						FCtrl: lorawan.FCtrl{
							ClassB: false,
						},
					},
				},
			},
			ExpectedBeaconLocked: false,
		},
	}

	for _, test := range testTable {
		ts.T().Run(test.Name, func(t *testing.T) {
			assert := require.New(t)

			if test.BeforeFunc != nil {
				test.BeforeFunc(&test.DeviceSession)
			}

			// create device-session
			assert.NoError(storage.SaveDeviceSession(ts.RedisPool(), test.DeviceSession))

			// set MIC
			assert.NoError(test.PHYPayload.SetUplinkDataMIC(lorawan.LoRaWAN1_0, 0, 0, 0, test.DeviceSession.FNwkSIntKey, test.DeviceSession.SNwkSIntKey))

			phyB, err := test.PHYPayload.MarshalBinary()
			assert.NoError(err)

			uplinkFrame := gw.UplinkFrame{
				PhyPayload: phyB,
				RxInfo:     &rxInfo,
				TxInfo:     &txInfo,
			}
			assert.NoError(uplink.HandleRXPacket(uplinkFrame))

			ds, err := storage.GetDeviceSession(ts.RedisPool(), test.DeviceSession.DevEUI)
			assert.NoError(err)

			assert.Equal(test.ExpectedBeaconLocked, ds.BeaconLocked)

			if test.ExpectedBeaconLocked {
				queueItems, err := storage.GetDeviceQueueItemsForDevEUI(config.C.PostgreSQL.DB, test.DeviceSession.DevEUI)
				assert.NoError(err)

				for _, qi := range queueItems {
					assert.NotNil(qi.EmitAtTimeSinceGPSEpoch)
					assert.NotNil(qi.TimeoutAfter)
				}
			}
		})
	}
}

func (ts *ClassBTestSuite) TestDownlink() {
	assert := require.New(ts.T())

	ds := storage.DeviceSession{
		RoutingProfileID: ts.Device.RoutingProfileID,
		ServiceProfileID: ts.Device.ServiceProfileID,
		DeviceProfileID:  ts.Device.DeviceProfileID,
		DevEUI:           ts.Device.DevEUI,
		DevAddr:          lorawan.DevAddr{1, 2, 3, 4},
		JoinEUI:          lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1},
		FNwkSIntKey:      lorawan.AES128Key{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SNwkSIntKey:      lorawan.AES128Key{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		NwkSEncKey:       lorawan.AES128Key{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		FCntUp:           8,
		NFCntDown:        5,
		UplinkGatewayHistory: map[lorawan.EUI64]storage.UplinkGatewayHistory{
			lorawan.EUI64{1, 2, 1, 2, 1, 2, 1, 2}: storage.UplinkGatewayHistory{},
		},
		EnabledUplinkChannels: []int{0, 1, 2},
		BeaconLocked:          true,
		PingSlotFrequency:     868300000,
		PingSlotDR:            2,
		RX2Frequency:          869525000,
	}

	emitTime := gps.Time(time.Now().Add(-10 * time.Second)).TimeSinceGPSEpoch()
	fPortTen := uint8(10)

	txInfo := gw.DownlinkTXInfo{
		GatewayId:         []byte{1, 2, 1, 2, 1, 2, 1, 2},
		Frequency:         uint32(ds.PingSlotFrequency),
		Power:             int32(config.C.NetworkServer.Band.Band.GetDownlinkTXPower(ds.PingSlotFrequency)),
		TimeSinceGpsEpoch: ptypes.DurationProto(emitTime),
	}
	assert.NoError(helpers.SetDownlinkTXInfoDataRate(&txInfo, 2, config.C.NetworkServer.Band.Band))

	txInfoDefaultFreq := txInfo
	txInfoDefaultFreq.Frequency = 869525000

	testTable := []struct {
		BeforeFunc       func(ds *storage.DeviceSession)
		Name             string
		DeviceSession    storage.DeviceSession
		DeviceQueueItems []storage.DeviceQueueItem

		ExpectedFCntUp     uint32
		ExpectedFCntDown   uint32
		ExpectedTXInfo     *gw.DownlinkTXInfo
		ExpectedPHYPayload *lorawan.PHYPayload
	}{
		{
			Name:          "class-b downlink",
			DeviceSession: ds,
			DeviceQueueItems: []storage.DeviceQueueItem{
				{DevEUI: ts.Device.DevEUI, FPort: 10, FCnt: 5, FRMPayload: []byte{1, 2, 3}, EmitAtTimeSinceGPSEpoch: &emitTime},
			},
			ExpectedFCntUp:   8,
			ExpectedFCntDown: 6,
			ExpectedTXInfo:   &txInfo,
			ExpectedPHYPayload: &lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataDown,
					Major: lorawan.LoRaWANR1,
				},
				MIC: lorawan.MIC{164, 6, 172, 129},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ds.DevAddr,
						FCnt:    5,
						FCtrl: lorawan.FCtrl{
							ADR: true,
						},
					},
					FPort: &fPortTen,
					FRMPayload: []lorawan.Payload{
						&lorawan.DataPayload{Bytes: []byte{1, 2, 3}},
					},
				},
			},
		},
		{
			Name:          "class-b downlink with more data",
			DeviceSession: ds,
			DeviceQueueItems: []storage.DeviceQueueItem{
				{DevEUI: ts.Device.DevEUI, FPort: 10, FCnt: 5, FRMPayload: []byte{1, 2, 3}, EmitAtTimeSinceGPSEpoch: &emitTime},
				{DevEUI: ts.Device.DevEUI, FPort: 10, FCnt: 6, FRMPayload: []byte{1, 2, 3}, EmitAtTimeSinceGPSEpoch: &emitTime},
			},
			ExpectedFCntUp:   8,
			ExpectedFCntDown: 6,
			ExpectedTXInfo:   &txInfo,
			ExpectedPHYPayload: &lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataDown,
					Major: lorawan.LoRaWANR1,
				},
				MIC: lorawan.MIC{39, 244, 225, 101},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ds.DevAddr,
						FCnt:    5,
						FCtrl: lorawan.FCtrl{
							ADR:      true,
							FPending: true,
							ClassB:   true, // shares the same bit as FPending
						},
					},
					FPort: &fPortTen,
					FRMPayload: []lorawan.Payload{
						&lorawan.DataPayload{Bytes: []byte{1, 2, 3}},
					},
				},
			},
		},
		{
			BeforeFunc: func(ds *storage.DeviceSession) {
				ds.BeaconLocked = false
			},
			Name:          "class-b downlink, but no beacon lock",
			DeviceSession: ds,
			DeviceQueueItems: []storage.DeviceQueueItem{
				{DevEUI: ts.Device.DevEUI, FPort: 10, FCnt: 5, FRMPayload: []byte{1, 2, 3}, EmitAtTimeSinceGPSEpoch: &emitTime},
			},
			ExpectedFCntUp:   8,
			ExpectedFCntDown: 5,
		},
		{
			BeforeFunc: func(ds *storage.DeviceSession) {
				config.C.NetworkServer.NetworkSettings.ClassB.PingSlotFrequency = 0
				ds.PingSlotFrequency = 0
			},
			Name:          "class-b downlink, with default band frequency plan",
			DeviceSession: ds,
			DeviceQueueItems: []storage.DeviceQueueItem{
				{DevEUI: ds.DevEUI, FPort: 10, FCnt: 5, FRMPayload: []byte{1, 2, 3}, EmitAtTimeSinceGPSEpoch: &emitTime},
			},
			ExpectedFCntUp:   8,
			ExpectedFCntDown: 6,
			ExpectedTXInfo:   &txInfoDefaultFreq,
			ExpectedPHYPayload: &lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataDown,
					Major: lorawan.LoRaWANR1,
				},
				MIC: lorawan.MIC{164, 6, 172, 129},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ds.DevAddr,
						FCnt:    5,
						FCtrl: lorawan.FCtrl{
							ADR: true,
						},
					},
					FPort: &fPortTen,
					FRMPayload: []lorawan.Payload{
						&lorawan.DataPayload{Bytes: []byte{1, 2, 3}},
					},
				},
			},
		},
	}

	for _, tst := range testTable {
		ts.T().Run(tst.Name, func(t *testing.T) {
			assert := require.New(t)
			assert.NoError(storage.FlushDeviceQueueForDevEUI(ts.DB(), tst.DeviceSession.DevEUI))

			if tst.BeforeFunc != nil {
				tst.BeforeFunc(&tst.DeviceSession)
			}
			assert.NoError(storage.SaveDeviceSession(ts.RedisPool(), tst.DeviceSession))
			for _, qi := range tst.DeviceQueueItems {
				assert.NoError(storage.CreateDeviceQueueItem(ts.DB(), &qi))
			}

			assert.NoError(downlink.ScheduleBatch(1))

			ds, err := storage.GetDeviceSession(ts.RedisPool(), tst.DeviceSession.DevEUI)
			assert.NoError(err)
			assert.Equal(tst.ExpectedFCntUp, ds.FCntUp)
			assert.Equal(tst.ExpectedFCntDown, ds.NFCntDown)

			if tst.ExpectedTXInfo != nil && tst.ExpectedPHYPayload != nil {
				testGW := config.C.NetworkServer.Gateway.Backend.Backend.(*test.GatewayBackend)
				downlinkFrame := <-testGW.TXPacketChan
				assert.NotEqual(0, downlinkFrame.Token)

				downlinkFrame.TxInfo.TimeSinceGpsEpoch.XXX_sizecache = 0
				modInfo := downlinkFrame.TxInfo.GetLoraModulationInfo()
				modInfo.XXX_sizecache = 0
				downlinkFrame.TxInfo.XXX_sizecache = 0

				assert.Equal(tst.ExpectedTXInfo, downlinkFrame.TxInfo)

				var phy lorawan.PHYPayload
				assert.NoError(phy.UnmarshalBinary(downlinkFrame.PhyPayload))

				assert.Equal(tst.ExpectedPHYPayload, &phy)
			}
		})
	}
}

func TestClassB(t *testing.T) {
	suite.Run(t, new(ClassBTestSuite))
}
