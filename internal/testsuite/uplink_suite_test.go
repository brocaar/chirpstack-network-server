package testsuite

import (
	"time"

	"github.com/gofrs/uuid"
	"github.com/stretchr/testify/suite"

	commonPB "github.com/brocaar/loraserver/api/common"
	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/loraserver/internal/test"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/band"
)

// UplinkIntegrationTestSuite provides a test-suite for integration-testing
// uplink scenarios.
type UplinkIntegrationTestSuite struct {
	suite.Suite

	// mocked interfaces
	ASClient  *test.ApplicationClient
	GWBackend *test.GatewayBackend

	// keys
	AppSKey lorawan.AES128Key

	// data objects
	RoutingProfile *storage.RoutingProfile
	ServiceProfile *storage.ServiceProfile
	DeviceProfile  *storage.DeviceProfile
	Device         *storage.Device
	DeviceSession  *storage.DeviceSession
	Gateway        *storage.Gateway
}

// SetupTest initializes the test-suite before running each test.
func (t *UplinkIntegrationTestSuite) SetupTest() {
	t.initConfig()
	t.FlushClients()
}

func (t *UplinkIntegrationTestSuite) initASClient() {
	t.ASClient = test.NewApplicationClient()
	config.C.ApplicationServer.Pool = test.NewApplicationServerPool(t.ASClient)
}

func (t *UplinkIntegrationTestSuite) initGWBackend() {
	t.GWBackend = test.NewGatewayBackend()
	config.C.NetworkServer.Gateway.Backend.Backend = t.GWBackend
}

func (t *UplinkIntegrationTestSuite) initNCClient() {
	config.C.NetworkController.Client = test.NewNetworkControllerClient()
}

func (t *UplinkIntegrationTestSuite) initConfig() {
	config.C.NetworkServer.DeviceSessionTTL = time.Hour
	config.C.NetworkServer.Band.Name = band.EU_863_870
	config.C.NetworkServer.Band.Band, _ = band.GetConfig(config.C.NetworkServer.Band.Name, false, lorawan.DwellTimeNoLimit)
	config.C.NetworkServer.DeduplicationDelay = 5 * time.Millisecond
	config.C.NetworkServer.GetDownlinkDataDelay = 5 * time.Millisecond
	config.C.NetworkServer.NetworkSettings.DownlinkTXPower = -1
	config.C.NetworkServer.NetworkSettings.RX2Frequency = config.C.NetworkServer.Band.Band.GetDefaults().RX2Frequency
	config.C.NetworkServer.NetworkSettings.RX2DR = config.C.NetworkServer.Band.Band.GetDefaults().RX2DataRate
	config.C.NetworkServer.NetworkSettings.RX1Delay = 0

	config.C.NetworkServer.Gateway.Stats.Timezone = "Europe/Amsterdam"
	loc, err := time.LoadLocation(config.C.NetworkServer.Gateway.Stats.Timezone)
	t.Nil(err)
	config.C.NetworkServer.Gateway.Stats.TimezoneLocation = loc

	testConfig := test.GetConfig()
	db, err := common.OpenDatabase(testConfig.PostgresDSN)
	t.Require().Nil(err)
	config.C.PostgreSQL.DB = db
	test.MustResetDB(db)

	p := common.NewRedisPool(testConfig.RedisURL, 10, 0)
	config.C.Redis.Pool = p
	test.MustFlushRedis(p)
	test.MustPrefillRedisPool(p, 1)
}

// CreateDeviceSession creates the given device-session.
func (t *UplinkIntegrationTestSuite) CreateDeviceSession(ds storage.DeviceSession) {
	var nilEUI lorawan.EUI64
	if ds.DevEUI == nilEUI {
		if t.Device == nil {
			t.CreateDevice(storage.Device{})
		}
		ds.DevEUI = t.Device.DevEUI
	}

	if ds.RoutingProfileID == uuid.Nil {
		if t.RoutingProfile == nil {
			t.CreateRoutingProfile(storage.RoutingProfile{})
		}
		ds.RoutingProfileID = t.RoutingProfile.ID
	}

	if ds.ServiceProfileID == uuid.Nil {
		if t.ServiceProfile == nil {
			t.CreateServiceProfile(storage.ServiceProfile{})
		}
		ds.ServiceProfileID = t.ServiceProfile.ID
	}

	if ds.DeviceProfileID == uuid.Nil {
		if t.DeviceProfile == nil {
			t.CreateDeviceProfile(storage.DeviceProfile{})
		}
		ds.DeviceProfileID = t.DeviceProfile.ID
	}

	t.Nil(storage.SaveDeviceSession(config.C.Redis.Pool, ds))
	t.DeviceSession = &ds
}

// CreateDevice creates the given device.
func (t *UplinkIntegrationTestSuite) CreateDevice(d storage.Device) {
	if d.DeviceProfileID == uuid.Nil {
		if t.DeviceProfile == nil {
			t.CreateDeviceProfile(storage.DeviceProfile{})
		}
		d.DeviceProfileID = t.DeviceProfile.ID
	}

	if d.ServiceProfileID == uuid.Nil {
		if t.ServiceProfile == nil {
			t.CreateServiceProfile(storage.ServiceProfile{})
		}
		d.ServiceProfileID = t.ServiceProfile.ID
	}

	if d.RoutingProfileID == uuid.Nil {
		if t.RoutingProfile == nil {
			t.CreateRoutingProfile(storage.RoutingProfile{})
		}
		d.RoutingProfileID = t.RoutingProfile.ID
	}

	t.Nil(storage.CreateDevice(config.C.PostgreSQL.DB, &d))
	t.Device = &d
}

// CreateDeviceProfile creates the given device-profile.
func (t *UplinkIntegrationTestSuite) CreateDeviceProfile(dp storage.DeviceProfile) {
	t.Require().Nil(storage.CreateDeviceProfile(config.C.PostgreSQL.DB, &dp))
	t.DeviceProfile = &dp
}

// CreateServiceProfile creates the given service-profile.
func (t *UplinkIntegrationTestSuite) CreateServiceProfile(sp storage.ServiceProfile) {
	t.Require().Nil(storage.CreateServiceProfile(config.C.PostgreSQL.DB, &sp))
	t.ServiceProfile = &sp
}

// CreateRoutingProfile creates the given routing-profile.
func (t *UplinkIntegrationTestSuite) CreateRoutingProfile(rp storage.RoutingProfile) {
	t.Require().Nil(storage.CreateRoutingProfile(config.C.PostgreSQL.DB, &rp))
	t.RoutingProfile = &rp
}

// CreateGateway creates the given gateway.
func (t *UplinkIntegrationTestSuite) CreateGateway(gw storage.Gateway) {
	t.Require().Nil(storage.CreateGateway(config.C.PostgreSQL.DB, &gw))
	t.Gateway = &gw
}

// GetUplinkFrameForFRMPayload returns the gateway uplink-frame for the given options.
func (t *UplinkIntegrationTestSuite) GetUplinkFrameForFRMPayload(rxInfo gw.UplinkRXInfo, txInfo gw.UplinkTXInfo, mType lorawan.MType, fPort uint8, data []byte, fOpts ...lorawan.Payload) gw.UplinkFrame {
	t.Require().NotNil(t.DeviceSession)
	t.Require().NotNil(t.Gateway)

	var err error
	var fCnt uint32
	var txChan int
	var txDR int

	txChan, err = config.C.NetworkServer.Band.Band.GetUplinkChannelIndex(int(txInfo.Frequency), true)
	t.Require().Nil(err)

	if txInfo.Modulation == commonPB.Modulation_LORA {
		modInfo := txInfo.GetLoraModulationInfo()
		t.Require().NotNil(modInfo)

		txDR, err = config.C.NetworkServer.Band.Band.GetDataRateIndex(true, band.DataRate{
			Modulation:   band.LoRaModulation,
			SpreadFactor: int(modInfo.SpreadingFactor),
			Bandwidth:    int(modInfo.Bandwidth),
		})
		t.Require().Nil(err)
	} else {
		modInfo := txInfo.GetFskModulationInfo()
		t.Require().NotNil(modInfo)

		txDR, err = config.C.NetworkServer.Band.Band.GetDataRateIndex(true, band.DataRate{
			Modulation: band.FSKModulation,
			BitRate:    int(modInfo.Bitrate),
		})
		t.Require().Nil(err)
	}

	if t.DeviceSession.GetMACVersion() == lorawan.LoRaWAN1_0 {
		fCnt = t.DeviceSession.NFCntDown
	} else {
		fCnt = t.DeviceSession.AFCntDown
	}

	phy := lorawan.PHYPayload{
		MHDR: lorawan.MHDR{
			MType: mType,
			Major: lorawan.LoRaWANR1,
		},
		MACPayload: &lorawan.MACPayload{
			FHDR: lorawan.FHDR{
				DevAddr: t.DeviceSession.DevAddr,
				FCtrl:   lorawan.FCtrl{},
				FCnt:    fCnt,
				FOpts:   fOpts,
			},
			FPort: &fPort,
			FRMPayload: []lorawan.Payload{
				&lorawan.DataPayload{Bytes: data},
			},
		},
	}
	t.Require().Nil(phy.EncryptFRMPayload(t.AppSKey))

	if t.DeviceSession.GetMACVersion() != lorawan.LoRaWAN1_0 {
		t.Require().Nil(phy.EncryptFOpts(t.DeviceSession.NwkSEncKey))
	}

	t.Require().Nil(phy.SetUplinkDataMIC(t.DeviceSession.GetMACVersion(), t.DeviceSession.ConfFCnt, uint8(txDR), uint8(txChan), t.DeviceSession.FNwkSIntKey, t.DeviceSession.SNwkSIntKey))

	b, err := phy.MarshalBinary()
	t.Require().Nil(err)

	return gw.UplinkFrame{
		RxInfo:     &rxInfo,
		TxInfo:     &txInfo,
		PhyPayload: b,
	}
}

// FlushClients flushes the GW, NC and AS clients to make sure all channels
// are empty. This is automatically called on SetupTest.
func (t *UplinkIntegrationTestSuite) FlushClients() {
	t.initASClient()
	t.initGWBackend()
	t.initNCClient()
}
