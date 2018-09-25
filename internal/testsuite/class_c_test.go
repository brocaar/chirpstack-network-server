package testsuite

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/downlink"
	"github.com/brocaar/loraserver/internal/helpers"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/lorawan"
)

type ClassCTestSuite struct {
	IntegrationTestSuite
}

func (ts *ClassCTestSuite) SetupTest() {
	ts.IntegrationTestSuite.SetupTest()
	config.C.NetworkServer.NetworkSettings.RX2DR = 5

	ts.CreateDeviceProfile(storage.DeviceProfile{SupportsClassC: true})

	// note that the CreateDeviceSession will automatically set
	// the device, profiles etc.. :)
	ds := storage.DeviceSession{
		FCntUp:    8,
		NFCntDown: 5,
		UplinkGatewayHistory: map[lorawan.EUI64]storage.UplinkGatewayHistory{
			lorawan.EUI64{1, 2, 1, 2, 1, 2, 1, 2}: storage.UplinkGatewayHistory{},
		},
		EnabledUplinkChannels: []int{0, 1, 2},
		RX2DR:                 5,
		RX2Frequency:          869525000,

		DevAddr:     lorawan.DevAddr{1, 2, 3, 4},
		FNwkSIntKey: lorawan.AES128Key{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SNwkSIntKey: lorawan.AES128Key{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		NwkSEncKey:  lorawan.AES128Key{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
	}
	ts.CreateDeviceSession(ds)
}

func (ts *ClassCTestSuite) TestClassC() {
	assert := require.New(ts.T())
	defaults := config.C.NetworkServer.Band.Band.GetDefaults()

	txInfo := gw.DownlinkTXInfo{
		GatewayId:   []byte{1, 2, 1, 2, 1, 2, 1, 2},
		Immediately: true,
		Frequency:   uint32(defaults.RX2Frequency),
		Power:       int32(config.C.NetworkServer.Band.Band.GetDownlinkTXPower(defaults.RX2Frequency)),
	}
	assert.NoError(helpers.SetDownlinkTXInfoDataRate(&txInfo, 5, config.C.NetworkServer.Band.Band))

	fPortTen := uint8(10)

	tests := []struct {
		Name             string
		BeforeFunc       func() error
		DeviceSession    storage.DeviceSession
		DeviceQueueItems []storage.DeviceQueueItem

		ExpectedFCntUp     uint32
		ExpectedFCntDown   uint32
		ExpectedTXInfo     *gw.DownlinkTXInfo
		ExpectedPHYPayload *lorawan.PHYPayload
	}{
		{
			Name:          "unconfirmed data",
			DeviceSession: *ts.DeviceSession,
			DeviceQueueItems: []storage.DeviceQueueItem{
				{DevEUI: ts.DeviceSession.DevEUI, FPort: 10, FCnt: 5, FRMPayload: make([]byte, 242)},
			},

			ExpectedFCntUp:   8,
			ExpectedFCntDown: 6,
			ExpectedTXInfo:   &txInfo,
			ExpectedPHYPayload: &lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataDown,
					Major: lorawan.LoRaWANR1,
				},
				MIC: lorawan.MIC{155, 150, 40, 188},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    5,
						FCtrl: lorawan.FCtrl{
							ADR: true,
						},
					},
					FPort: &fPortTen,
					FRMPayload: []lorawan.Payload{
						&lorawan.DataPayload{Bytes: make([]byte, 242)},
					},
				},
			},
		},
		{
			Name:          "unconfirmed data (only first item is emitted because of class-c downlink lock)",
			DeviceSession: *ts.DeviceSession,
			DeviceQueueItems: []storage.DeviceQueueItem{
				{DevEUI: ts.DeviceSession.DevEUI, FPort: 10, FCnt: 5, FRMPayload: make([]byte, 242)},
				{DevEUI: ts.DeviceSession.DevEUI, FPort: 10, FCnt: 6, FRMPayload: make([]byte, 242)},
			},
			ExpectedFCntUp:   8,
			ExpectedFCntDown: 6,
			ExpectedTXInfo:   &txInfo,
			ExpectedPHYPayload: &lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataDown,
					Major: lorawan.LoRaWANR1,
				},
				MIC: lorawan.MIC{166, 225, 232, 165},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    5,
						FCtrl: lorawan.FCtrl{
							ADR:      true,
							FPending: true,
							ClassB:   true, // shares the same bit as FPending
						},
					},
					FPort: &fPortTen,
					FRMPayload: []lorawan.Payload{
						&lorawan.DataPayload{Bytes: make([]byte, 242)},
					},
				},
			},
		},
		{
			Name:          "confirmed data",
			DeviceSession: *ts.DeviceSession,
			DeviceQueueItems: []storage.DeviceQueueItem{
				{DevEUI: ts.DeviceSession.DevEUI, FPort: 10, FCnt: 5, Confirmed: true, FRMPayload: []byte{5, 4, 3, 2, 1}},
			},

			ExpectedFCntUp:   8,
			ExpectedFCntDown: 6,
			ExpectedTXInfo:   &txInfo,
			ExpectedPHYPayload: &lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.ConfirmedDataDown,
					Major: lorawan.LoRaWANR1,
				},
				MIC: lorawan.MIC{212, 125, 174, 208},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    5,
						FCtrl: lorawan.FCtrl{
							ADR: true,
						},
					},
					FPort: &fPortTen,
					FRMPayload: []lorawan.Payload{
						&lorawan.DataPayload{Bytes: []byte{5, 4, 3, 2, 1}},
					},
				},
			},
		},
		{
			Name:          "queue item discarded (max payload exceeded)",
			DeviceSession: *ts.DeviceSession,
			DeviceQueueItems: []storage.DeviceQueueItem{
				{DevEUI: ts.DeviceSession.DevEUI, FPort: 10, FCnt: 10, FRMPayload: make([]byte, 300)},
			},
			ExpectedFCntUp:   8,
			ExpectedFCntDown: 5,
		},
		{
			Name: "containing mac-commands",
			BeforeFunc: func() error {
				ts.ServiceProfile.DevStatusReqFreq = 1
				if err := storage.UpdateServiceProfile(config.C.PostgreSQL.DB, ts.ServiceProfile); err != nil {
					return err
				}
				if err := storage.FlushServiceProfileCache(config.C.Redis.Pool, ts.ServiceProfile.ID); err != nil {
					return err
				}

				return nil
			},
			DeviceSession: *ts.DeviceSession,
			DeviceQueueItems: []storage.DeviceQueueItem{
				{DevEUI: ts.DeviceSession.DevEUI, FPort: 10, FCnt: 5, FRMPayload: []byte{5, 4, 3, 2, 1}},
			},

			ExpectedFCntUp:   8,
			ExpectedFCntDown: 6,
			ExpectedTXInfo:   &txInfo,
			ExpectedPHYPayload: &lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataDown,
					Major: lorawan.LoRaWANR1,
				},
				MIC: lorawan.MIC{115, 18, 33, 93},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: ts.DeviceSession.DevAddr,
						FCnt:    5,
						FCtrl: lorawan.FCtrl{
							ADR: true,
						},
						FOpts: []lorawan.Payload{
							&lorawan.MACCommand{CID: lorawan.CID(6)},
						},
					},
					FPort: &fPortTen,
					FRMPayload: []lorawan.Payload{
						&lorawan.DataPayload{Bytes: []byte{5, 4, 3, 2, 1}},
					},
				},
			},
		},
	}

	for _, tst := range tests {
		ts.T().Run(tst.Name, func(t *testing.T) {
			assert := require.New(t)

			if tst.BeforeFunc != nil {
				assert.NoError(tst.BeforeFunc())
			}

			ts.FlushClients()

			// overwrite device-session to deal with frame-counter increments
			ts.CreateDeviceSession(tst.DeviceSession)

			// add device-queue items
			assert.NoError(storage.FlushDeviceQueueForDevEUI(config.C.PostgreSQL.DB, tst.DeviceSession.DevEUI))
			for _, qi := range tst.DeviceQueueItems {
				assert.NoError(storage.CreateDeviceQueueItem(config.C.PostgreSQL.DB, &qi))
			}

			// run queue scheduler
			assert.NoError(downlink.ScheduleDeviceQueueBatch(1))

			// test frame-counters
			ds, err := storage.GetDeviceSession(config.C.Redis.Pool, tst.DeviceSession.DevEUI)
			assert.NoError(err)
			assert.Equal(tst.ExpectedFCntUp, ds.FCntUp)
			assert.Equal(tst.ExpectedFCntDown, ds.NFCntDown)

			if tst.ExpectedTXInfo != nil && tst.ExpectedPHYPayload != nil {
				downlinkFrame := <-ts.GWBackend.TXPacketChan
				assert.NotEqual(0, downlinkFrame.Token)
				if !proto.Equal(downlinkFrame.TxInfo, tst.ExpectedTXInfo) {
					assert.Equal(tst.ExpectedTXInfo, downlinkFrame.TxInfo)
				}

				b, err := tst.ExpectedPHYPayload.MarshalBinary()
				assert.NoError(err)
				assert.NoError(tst.ExpectedPHYPayload.UnmarshalBinary(b))
				assert.NoError(tst.ExpectedPHYPayload.DecodeFOptsToMACCommands())

				var phy lorawan.PHYPayload
				assert.NoError(phy.UnmarshalBinary(downlinkFrame.PhyPayload))
				assert.NoError(phy.DecodeFOptsToMACCommands())
				assert.Equal(tst.ExpectedPHYPayload, &phy)
			} else {
				assert.Equal(0, len(ts.GWBackend.TXPacketChan))
			}
		})
	}
}

func TestClassCNew(t *testing.T) {
	suite.Run(t, new(ClassCTestSuite))
}
