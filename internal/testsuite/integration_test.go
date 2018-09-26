package testsuite

import (
	"testing"
	"time"

	"github.com/gofrs/uuid"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	commonPB "github.com/brocaar/loraserver/api/common"
	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/downlink"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/loraserver/internal/test"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/band"
)

// Assertion provides the interface for test assertions.
type Assertion func(*require.Assertions, *IntegrationTestSuite)

// DownlinkTest is the structure for a downlink test.
type DownlinkTest struct {
	Name             string
	BeforeFunc       func(*DownlinkTest) error
	DeviceSession    storage.DeviceSession
	DeviceQueueItems []storage.DeviceQueueItem

	Assert []Assertion
}

// MulticastTest is the structure for a multicast test.
type MulticastTest struct {
	Name                string
	BeforeFunc          func(*MulticastTest) error
	MulticastGroup      storage.MulticastGroup
	MulticastQueueItems []storage.MulticastQueueItem

	Assert []Assertion
}

// IntegrationTestSuite provides a test-suite for integration-testing
// uplink scenarios.
type IntegrationTestSuite struct {
	suite.Suite
	test.DatabaseTestSuiteBase

	// mocked interfaces
	ASClient  *test.ApplicationClient
	GWBackend *test.GatewayBackend
	GeoClient *test.GeolocationClient

	// keys
	AppSKey lorawan.AES128Key

	// data objects
	RoutingProfile *storage.RoutingProfile
	ServiceProfile *storage.ServiceProfile
	DeviceProfile  *storage.DeviceProfile
	Device         *storage.Device
	DeviceSession  *storage.DeviceSession
	Gateway        *storage.Gateway
	MulticastGroup *storage.MulticastGroup
}

// SetupTest initializes the test-suite before running each test.
func (ts *IntegrationTestSuite) SetupTest() {
	ts.DatabaseTestSuiteBase.SetupTest()
	ts.initConfig()
	ts.FlushClients()
}

// FlushClients flushes the GW, NC and AS clients to make sure all channels
// are empty. This is automatically called on SetupTest.
func (ts *IntegrationTestSuite) FlushClients() {
	ts.ASClient = test.NewApplicationClient()
	config.C.ApplicationServer.Pool = test.NewApplicationServerPool(ts.ASClient)

	ts.GWBackend = test.NewGatewayBackend()
	config.C.NetworkServer.Gateway.Backend.Backend = ts.GWBackend

	config.C.NetworkController.Client = test.NewNetworkControllerClient()

	ts.GeoClient = test.NewGeolocationClient()
	config.C.GeolocationServer.Client = ts.GeoClient
}

// CreateDeviceSession creates the given device-session.
func (ts *IntegrationTestSuite) CreateDeviceSession(ds storage.DeviceSession) {
	var nilEUI lorawan.EUI64
	if ds.DevEUI == nilEUI {
		if ts.Device == nil {
			ts.CreateDevice(storage.Device{})
		}
		ds.DevEUI = ts.Device.DevEUI
	}

	if ds.RoutingProfileID == uuid.Nil {
		if ts.RoutingProfile == nil {
			ts.CreateRoutingProfile(storage.RoutingProfile{})
		}
		ds.RoutingProfileID = ts.RoutingProfile.ID
	}

	if ds.ServiceProfileID == uuid.Nil {
		if ts.ServiceProfile == nil {
			ts.CreateServiceProfile(storage.ServiceProfile{})
		}
		ds.ServiceProfileID = ts.ServiceProfile.ID
	}

	if ds.DeviceProfileID == uuid.Nil {
		if ts.DeviceProfile == nil {
			ts.CreateDeviceProfile(storage.DeviceProfile{})
		}
		ds.DeviceProfileID = ts.DeviceProfile.ID
	}

	ts.Nil(storage.SaveDeviceSession(ts.RedisPool(), ds))
	ts.DeviceSession = &ds
}

// CreateDevice creates the given device.
func (ts *IntegrationTestSuite) CreateDevice(d storage.Device) {
	if d.DeviceProfileID == uuid.Nil {
		if ts.DeviceProfile == nil {
			ts.CreateDeviceProfile(storage.DeviceProfile{})
		}
		d.DeviceProfileID = ts.DeviceProfile.ID
	}

	if d.ServiceProfileID == uuid.Nil {
		if ts.ServiceProfile == nil {
			ts.CreateServiceProfile(storage.ServiceProfile{})
		}
		d.ServiceProfileID = ts.ServiceProfile.ID
	}

	if d.RoutingProfileID == uuid.Nil {
		if ts.RoutingProfile == nil {
			ts.CreateRoutingProfile(storage.RoutingProfile{})
		}
		d.RoutingProfileID = ts.RoutingProfile.ID
	}

	ts.Nil(storage.CreateDevice(config.C.PostgreSQL.DB, &d))
	ts.Device = &d
}

// CreateMulticastGroup creates the given multicast-group.
func (ts *IntegrationTestSuite) CreateMulticastGroup(mg storage.MulticastGroup) {
	if mg.RoutingProfileID == uuid.Nil {
		if ts.RoutingProfile == nil {
			ts.CreateRoutingProfile(storage.RoutingProfile{})
		}
		mg.RoutingProfileID = ts.RoutingProfile.ID
	}

	if mg.ServiceProfileID == uuid.Nil {
		if ts.ServiceProfile == nil {
			ts.CreateServiceProfile(storage.ServiceProfile{})
		}
		mg.ServiceProfileID = ts.ServiceProfile.ID
	}

	ts.Nil(storage.CreateMulticastGroup(config.C.PostgreSQL.DB, &mg))
	ts.MulticastGroup = &mg
}

// CreateDeviceProfile creates the given device-profile.
func (ts *IntegrationTestSuite) CreateDeviceProfile(dp storage.DeviceProfile) {
	ts.Require().Nil(storage.CreateDeviceProfile(ts.DB(), &dp))
	ts.DeviceProfile = &dp
}

// CreateServiceProfile creates the given service-profile.
func (ts *IntegrationTestSuite) CreateServiceProfile(sp storage.ServiceProfile) {
	ts.Require().Nil(storage.CreateServiceProfile(ts.DB(), &sp))
	ts.ServiceProfile = &sp
}

// CreateRoutingProfile creates the given routing-profile.
func (ts *IntegrationTestSuite) CreateRoutingProfile(rp storage.RoutingProfile) {
	ts.Require().Nil(storage.CreateRoutingProfile(ts.DB(), &rp))
	ts.RoutingProfile = &rp
}

// CreateGateway creates the given gateway.
func (ts *IntegrationTestSuite) CreateGateway(gw storage.Gateway) {
	ts.Require().Nil(storage.CreateGateway(ts.DB(), &gw))
	ts.Gateway = &gw
}

// GetUplinkFrameForFRMPayload returns the gateway uplink-frame for the given options.
func (ts *IntegrationTestSuite) GetUplinkFrameForFRMPayload(rxInfo gw.UplinkRXInfo, txInfo gw.UplinkTXInfo, mType lorawan.MType, fPort uint8, data []byte, fOpts ...lorawan.Payload) gw.UplinkFrame {
	ts.Require().NotNil(ts.DeviceSession)

	var err error
	var txChan int
	var txDR int

	txChan, err = config.C.NetworkServer.Band.Band.GetUplinkChannelIndex(int(txInfo.Frequency), true)
	ts.Require().Nil(err)

	if txInfo.Modulation == commonPB.Modulation_LORA {
		modInfo := txInfo.GetLoraModulationInfo()
		ts.Require().NotNil(modInfo)

		txDR, err = config.C.NetworkServer.Band.Band.GetDataRateIndex(true, band.DataRate{
			Modulation:   band.LoRaModulation,
			SpreadFactor: int(modInfo.SpreadingFactor),
			Bandwidth:    int(modInfo.Bandwidth),
		})
		ts.Require().Nil(err)
	} else {
		modInfo := txInfo.GetFskModulationInfo()
		ts.Require().NotNil(modInfo)

		txDR, err = config.C.NetworkServer.Band.Band.GetDataRateIndex(true, band.DataRate{
			Modulation: band.FSKModulation,
			BitRate:    int(modInfo.Bitrate),
		})
		ts.Require().Nil(err)
	}

	phy := lorawan.PHYPayload{
		MHDR: lorawan.MHDR{
			MType: mType,
			Major: lorawan.LoRaWANR1,
		},
		MACPayload: &lorawan.MACPayload{
			FHDR: lorawan.FHDR{
				DevAddr: ts.DeviceSession.DevAddr,
				FCtrl:   lorawan.FCtrl{},
				FCnt:    ts.DeviceSession.FCntUp,
				FOpts:   fOpts,
			},
			FPort: &fPort,
			FRMPayload: []lorawan.Payload{
				&lorawan.DataPayload{Bytes: data},
			},
		},
	}
	ts.Require().Nil(phy.EncryptFRMPayload(ts.AppSKey))

	if ts.DeviceSession.GetMACVersion() != lorawan.LoRaWAN1_0 {
		ts.Require().Nil(phy.EncryptFOpts(ts.DeviceSession.NwkSEncKey))
	}

	ts.Require().Nil(phy.SetUplinkDataMIC(ts.DeviceSession.GetMACVersion(), ts.DeviceSession.ConfFCnt, uint8(txDR), uint8(txChan), ts.DeviceSession.FNwkSIntKey, ts.DeviceSession.SNwkSIntKey))

	b, err := phy.MarshalBinary()
	ts.Require().Nil(err)

	return gw.UplinkFrame{
		RxInfo:     &rxInfo,
		TxInfo:     &txInfo,
		PhyPayload: b,
	}
}

// AssertDownlinkTest asserts the given downlink test.
func (ts *IntegrationTestSuite) AssertDownlinkTest(t *testing.T, tst DownlinkTest) {
	assert := require.New(t)

	if tst.BeforeFunc != nil {
		assert.NoError(tst.BeforeFunc(&tst))
	}

	ts.FlushClients()

	// overwrite device-session to deal with frame-counter increments
	ts.CreateDeviceSession(tst.DeviceSession)

	// add device-queue items
	assert.NoError(storage.FlushDeviceQueueForDevEUI(ts.DB(), tst.DeviceSession.DevEUI))
	for _, qi := range tst.DeviceQueueItems {
		assert.NoError(storage.CreateDeviceQueueItem(ts.DB(), &qi))
	}

	// run queue scheduler
	assert.NoError(downlink.ScheduleDeviceQueueBatch(1))

	// refresh device-session
	var err error
	ds, err := storage.GetDeviceSession(ts.RedisPool(), ts.DeviceSession.DevEUI)
	assert.NoError(err)
	ts.DeviceSession = &ds

	// run assertions
	for _, a := range tst.Assert {
		a(assert, ts)
	}
}

// AssertMulticastTest asserts the given multicast test.
func (ts *IntegrationTestSuite) AssertMulticastTest(t *testing.T, tst MulticastTest) {
	assert := require.New(t)

	if tst.BeforeFunc != nil {
		assert.NoError(tst.BeforeFunc(&tst))
	}

	ts.FlushClients()

	// overwrite multicast-group to deal with frame-counter increments
	assert.NoError(storage.UpdateMulticastGroup(ts.DB(), &tst.MulticastGroup))

	// add multicast queue items
	assert.NoError(storage.FlushMulticastQueueForMulticastGroup(ts.DB(), ts.MulticastGroup.ID))
	for _, qi := range tst.MulticastQueueItems {
		assert.NoError(storage.CreateMulticastQueueItem(ts.DB(), &qi))
	}

	// run multicast scheduler
	assert.NoError(downlink.ScheduleMulticastQueueBatch(1))

	// run assertions
	for _, a := range tst.Assert {
		a(assert, ts)
	}
}

func (ts *IntegrationTestSuite) initConfig() {
	config.C.NetworkServer.DeviceSessionTTL = time.Hour
	config.C.NetworkServer.Band.Name = band.EU_863_870
	config.C.NetworkServer.Band.Band, _ = band.GetConfig(config.C.NetworkServer.Band.Name, false, lorawan.DwellTimeNoLimit)
	config.C.NetworkServer.DeduplicationDelay = 100 * time.Millisecond
	config.C.NetworkServer.GetDownlinkDataDelay = 5 * time.Millisecond
	config.C.NetworkServer.NetworkSettings.DownlinkTXPower = -1
	config.C.NetworkServer.NetworkSettings.RX2Frequency = config.C.NetworkServer.Band.Band.GetDefaults().RX2Frequency
	config.C.NetworkServer.NetworkSettings.RX2DR = config.C.NetworkServer.Band.Band.GetDefaults().RX2DataRate
	config.C.NetworkServer.NetworkSettings.RX1Delay = 0

	config.C.NetworkServer.Gateway.Stats.Timezone = "Europe/Amsterdam"
	loc, err := time.LoadLocation(config.C.NetworkServer.Gateway.Stats.Timezone)
	ts.Nil(err)
	config.C.NetworkServer.Gateway.Stats.TimezoneLocation = loc
}
