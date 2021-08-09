package testsuite

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/brocaar/chirpstack-network-server/v3/internal/band"
	"github.com/brocaar/chirpstack-network-server/v3/internal/downlink"
	"github.com/brocaar/chirpstack-network-server/v3/internal/helpers"
	"github.com/brocaar/chirpstack-network-server/v3/internal/storage"
	"github.com/brocaar/chirpstack-network-server/v3/internal/test"
	"github.com/brocaar/lorawan"
)

type ClassCTestSuite struct {
	IntegrationTestSuite
}

func (ts *ClassCTestSuite) SetupTest() {
	assert := require.New(ts.T())
	ts.IntegrationTestSuite.SetupTest()

	conf := test.GetConfig()
	conf.NetworkServer.NetworkSettings.RX2DR = 5
	assert.NoError(downlink.Setup(conf))

	ts.CreateDeviceProfile(storage.DeviceProfile{SupportsClassC: true})

	ts.CreateDevice(storage.Device{
		Mode: storage.DeviceModeC,
	})

	ts.CreateGateway(storage.Gateway{})

	// note that the CreateDeviceSession will automatically set
	// the device, profiles etc.. :)
	ds := storage.DeviceSession{
		FCntUp:                8,
		NFCntDown:             5,
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
	defaults := band.Band().GetDefaults()

	txInfo := gw.DownlinkTXInfo{
		Board:     1,
		Antenna:   2,
		Frequency: uint32(defaults.RX2Frequency),
		Power:     int32(band.Band().GetDownlinkTXPower(defaults.RX2Frequency)),
		Timing:    gw.DownlinkTiming_IMMEDIATELY,
		TimingInfo: &gw.DownlinkTXInfo_ImmediatelyTimingInfo{
			ImmediatelyTimingInfo: &gw.ImmediatelyTimingInfo{},
		},
	}
	assert.NoError(helpers.SetDownlinkTXInfoDataRate(&txInfo, 5, band.Band()))

	fPortTen := uint8(10)

	tests := []DownlinkTest{
		{
			Name:          "unconfirmed data",
			DeviceSession: *ts.DeviceSession,
			DeviceQueueItems: []storage.DeviceQueueItem{
				{DevEUI: ts.DeviceSession.DevEUI, FPort: 10, FCnt: 5, FRMPayload: make([]byte, 242)},
			},
			Assert: []Assertion{
				AssertFCntUp(8),
				AssertNFCntDown(5),
				AssertDownlinkFrame(ts.Gateway.GatewayID, txInfo, lorawan.PHYPayload{
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
				}),
				AssertDownlinkGatewayLock(ts.Gateway.GatewayID),
			},
		},
		{
			Name:          "unconfirmed data (only first item is emitted because of class-c downlink lock)",
			DeviceSession: *ts.DeviceSession,
			DeviceQueueItems: []storage.DeviceQueueItem{
				{DevEUI: ts.DeviceSession.DevEUI, FPort: 10, FCnt: 5, FRMPayload: make([]byte, 242)},
				{DevEUI: ts.DeviceSession.DevEUI, FPort: 10, FCnt: 6, FRMPayload: make([]byte, 242)},
			},
			Assert: []Assertion{
				AssertFCntUp(8),
				AssertNFCntDown(5),
				AssertDownlinkFrame(ts.Gateway.GatewayID, txInfo, lorawan.PHYPayload{
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
				}),
			},
		},
		{
			Name:          "confirmed data",
			DeviceSession: *ts.DeviceSession,
			DeviceQueueItems: []storage.DeviceQueueItem{
				{DevEUI: ts.DeviceSession.DevEUI, FPort: 10, FCnt: 5, Confirmed: true, FRMPayload: []byte{5, 4, 3, 2, 1}},
			},
			Assert: []Assertion{
				AssertFCntUp(8),
				AssertNFCntDown(5),
				AssertDownlinkFrame(ts.Gateway.GatewayID, txInfo, lorawan.PHYPayload{
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
				}),
			},
		},
		{
			Name:          "queue item discarded (max payload exceeded)",
			DeviceSession: *ts.DeviceSession,
			DeviceQueueItems: []storage.DeviceQueueItem{
				{DevEUI: ts.DeviceSession.DevEUI, FPort: 10, FCnt: 10, FRMPayload: make([]byte, 300)},
			},
			Assert: []Assertion{
				AssertFCntUp(8),
				AssertNFCntDown(5),
				AssertNoDownlinkFrame,
			},
		},
	}

	for _, tst := range tests {
		ts.T().Run(tst.Name, func(t *testing.T) {
			ts.AssertDownlinkTest(t, tst)
		})
	}
}

func TestClassC(t *testing.T) {
	suite.Run(t, new(ClassCTestSuite))
}
