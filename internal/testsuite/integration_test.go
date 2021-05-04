package testsuite

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofrs/uuid"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/brocaar/chirpstack-api/go/v3/common"
	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/brocaar/chirpstack-api/go/v3/ns"
	nsapi "github.com/brocaar/chirpstack-network-server/v3/internal/api/ns"
	"github.com/brocaar/chirpstack-network-server/v3/internal/backend/applicationserver"
	"github.com/brocaar/chirpstack-network-server/v3/internal/backend/controller"
	"github.com/brocaar/chirpstack-network-server/v3/internal/backend/gateway"
	"github.com/brocaar/chirpstack-network-server/v3/internal/backend/joinserver"
	"github.com/brocaar/chirpstack-network-server/v3/internal/band"
	"github.com/brocaar/chirpstack-network-server/v3/internal/downlink"
	"github.com/brocaar/chirpstack-network-server/v3/internal/downlink/ack"
	"github.com/brocaar/chirpstack-network-server/v3/internal/storage"
	"github.com/brocaar/chirpstack-network-server/v3/internal/test"
	"github.com/brocaar/chirpstack-network-server/v3/internal/uplink"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/backend"
	loraband "github.com/brocaar/lorawan/band"
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

// OTAATest is the structure for an OTAA test.
type OTAATest struct {
	Name                     string
	BeforeFunc               func(*OTAATest) error
	AfterFunc                func(*OTAATest) error
	TXInfo                   gw.UplinkTXInfo
	RXInfo                   gw.UplinkRXInfo
	PHYPayload               lorawan.PHYPayload
	JoinServerJoinAnsPayload backend.JoinAnsPayload
	ExtraChannels            []int
	DeviceActivations        []storage.DeviceActivation
	DeviceQueueItems         []storage.DeviceQueueItem

	ExpectedError error
	Assert        []Assertion
}

// RejoinTest is the structure for a rejoin test.
type RejoinTest struct {
	Name                            string
	BeforeFunc                      func(*RejoinTest) error
	AfterFunc                       func(*RejoinTest) error
	TXInfo                          gw.UplinkTXInfo
	RXInfo                          gw.UplinkRXInfo
	PHYPayload                      lorawan.PHYPayload
	DeviceSession                   storage.DeviceSession
	JoinServerRejoinAnsPayload      backend.RejoinAnsPayload
	JoinServerRejoinAnsPayloadError error

	ExpectedError error
	Assert        []Assertion
}

// DownlinkProprietaryTest is the structure for a downlink proprietary test.
type DownlinkProprietaryTest struct {
	Name                          string
	SendProprietaryPayloadRequest ns.SendProprietaryPayloadRequest

	Assert []Assertion
}

// UplinkProprietaryTest is the structure for an uplink proprietary test.
type UplinkProprietaryTest struct {
	Name       string
	PHYPayload lorawan.PHYPayload
	TXInfo     gw.UplinkTXInfo
	RXInfo     gw.UplinkRXInfo

	Assert []Assertion
}

// ClassATest is the structure for a Class-A test.
type ClassATest struct {
	Name                    string
	PHYPayload              lorawan.PHYPayload
	TXInfo                  gw.UplinkTXInfo
	RXInfo                  gw.UplinkRXInfo
	BeforeFunc              func(*ClassATest) error
	AfterFunc               func(*ClassATest) error
	DeviceSession           storage.DeviceSession
	DeviceQueueItems        []storage.DeviceQueueItem
	MACCommandQueueItems    []storage.MACCommandBlock
	PendingMACCommands      []storage.MACCommandBlock
	ASHandleUplinkDataError error

	Assert        []Assertion
	ExpectedError error
}

// DownlinkTXAckTest is the structure for a downlink tx ack test.
type DownlinkTXAckTest struct {
	Name                string
	DeviceSession       storage.DeviceSession
	DownlinkTXAck       *gw.DownlinkTXAck
	DownlinkFrame       *storage.DownlinkFrame
	DeviceQueueItems    []storage.DeviceQueueItem
	MulticastQueueItems []storage.MulticastQueueItem

	Assert        []Assertion
	ExpectedError error
}

// IntegrationTestSuite provides a test-suite for integration-testing
// uplink scenarios.
type IntegrationTestSuite struct {
	suite.Suite

	// mocked interfaces
	ASClient  *test.ApplicationClient
	GWBackend *test.GatewayBackend
	NCClient  *test.NetworkControllerClient
	NSAPI     ns.NetworkServerServiceServer

	// keys
	JoinAcceptKey lorawan.AES128Key
	AppSKey       lorawan.AES128Key

	// data objects
	RoutingProfile *storage.RoutingProfile
	ServiceProfile *storage.ServiceProfile
	DeviceProfile  *storage.DeviceProfile
	Device         *storage.Device
	DeviceSession  *storage.DeviceSession
	Gateway        *storage.Gateway
	MulticastGroup *storage.MulticastGroup

	backendAPIServer   *httptest.Server
	backendAPIRequest  chan []byte
	backendAPIResponse backend.Answer
}

func (ts *IntegrationTestSuite) backendAPIHandler(w http.ResponseWriter, r *http.Request) {
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		panic(err)
	}

	ts.backendAPIRequest <- b

	b, _ = json.Marshal(ts.backendAPIResponse)
	w.Write(b)
}

func (ts *IntegrationTestSuite) SetupSuite() {
	assert := require.New(ts.T())
	conf := test.GetConfig()
	assert.NoError(storage.Setup(conf))
	assert.NoError(storage.MigrateDown(storage.DB().DB))
	assert.NoError(storage.MigrateUp(storage.DB().DB))

	ts.backendAPIServer = httptest.NewServer(http.HandlerFunc(ts.backendAPIHandler))

	conf.JoinServer.Default.Server = ts.backendAPIServer.URL
	assert.NoError(joinserver.Setup(conf))
}

// SetupTest initializes the test-suite before running each test.
func (ts *IntegrationTestSuite) SetupTest() {
	storage.RedisClient().FlushAll()
	ts.initConfig()
	ts.FlushClients()
}

// FlushClients flushes the GW, NC and AS clients to make sure all channels
// are empty. This is automatically called on SetupTest.
func (ts *IntegrationTestSuite) FlushClients() {
	ts.ASClient = test.NewApplicationClient()
	applicationserver.SetPool(test.NewApplicationServerPool(ts.ASClient))

	ts.GWBackend = test.NewGatewayBackend()
	gateway.SetBackend(ts.GWBackend)

	ts.NCClient = test.NewNetworkControllerClient()
	controller.SetClient(ts.NCClient)

	ts.NSAPI = nsapi.NewNetworkServerAPI()

	ts.backendAPIRequest = make(chan []byte, 1)
	ts.backendAPIResponse = nil
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

	ts.Nil(storage.SaveDeviceSession(context.Background(), ds))
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

	ts.Nil(storage.CreateDevice(context.Background(), storage.DB(), &d))
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

	ts.Nil(storage.CreateMulticastGroup(context.Background(), storage.DB(), &mg))
	ts.MulticastGroup = &mg
}

// CreateDeviceProfile creates the given device-profile.
func (ts *IntegrationTestSuite) CreateDeviceProfile(dp storage.DeviceProfile) {
	ts.Require().Nil(storage.CreateDeviceProfile(context.Background(), storage.DB(), &dp))
	ts.DeviceProfile = &dp
}

// CreateServiceProfile creates the given service-profile.
func (ts *IntegrationTestSuite) CreateServiceProfile(sp storage.ServiceProfile) {
	ts.Require().Nil(storage.CreateServiceProfile(context.Background(), storage.DB(), &sp))
	ts.ServiceProfile = &sp
}

// CreateRoutingProfile creates the given routing-profile.
func (ts *IntegrationTestSuite) CreateRoutingProfile(rp storage.RoutingProfile) {
	ts.Require().Nil(storage.CreateRoutingProfile(context.Background(), storage.DB(), &rp))
	ts.RoutingProfile = &rp
}

// CreateGateway creates the given gateway.
func (ts *IntegrationTestSuite) CreateGateway(gw storage.Gateway) {
	if gw.RoutingProfileID == uuid.Nil {
		if ts.RoutingProfile == nil {
			ts.CreateRoutingProfile(storage.RoutingProfile{})
		}

		gw.RoutingProfileID = ts.RoutingProfile.ID
	}

	if gw.ServiceProfileID == nil {
		if ts.ServiceProfile == nil {
			ts.CreateServiceProfile(storage.ServiceProfile{})
		}

		gw.ServiceProfileID = &ts.ServiceProfile.ID
	}

	ts.Require().Nil(storage.CreateGateway(context.Background(), storage.DB(), &gw))
	ts.Gateway = &gw
}

// GetUplinkFrameForFRMPayload returns the gateway uplink-frame for the given options.
func (ts *IntegrationTestSuite) GetUplinkFrameForFRMPayload(rxInfo gw.UplinkRXInfo, txInfo gw.UplinkTXInfo, mType lorawan.MType, fPort uint8, data []byte, fOpts ...lorawan.Payload) gw.UplinkFrame {
	ts.Require().NotNil(ts.DeviceSession)

	var err error
	var txChan int
	var txDR int

	txChan, err = band.Band().GetUplinkChannelIndex(int(txInfo.Frequency), true)
	ts.Require().Nil(err)

	if txInfo.Modulation == common.Modulation_LORA {
		modInfo := txInfo.GetLoraModulationInfo()
		ts.Require().NotNil(modInfo)

		txDR, err = band.Band().GetDataRateIndex(true, loraband.DataRate{
			Modulation:   loraband.LoRaModulation,
			SpreadFactor: int(modInfo.SpreadingFactor),
			Bandwidth:    int(modInfo.Bandwidth),
		})
		ts.Require().Nil(err)
	} else {
		modInfo := txInfo.GetFskModulationInfo()
		ts.Require().NotNil(modInfo)

		txDR, err = band.Band().GetDataRateIndex(true, loraband.DataRate{
			Modulation: loraband.FSKModulation,
			BitRate:    int(modInfo.Datarate),
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
	assert.NoError(storage.RedisClient().FlushAll().Err())

	ts.CreateDeviceSession(tst.DeviceSession)

	// set downlink path
	deviceGatewayRXInfoSet := storage.DeviceGatewayRXInfoSet{
		DevEUI: ts.Device.DevEUI,
		DR:     0,
		Items: []storage.DeviceGatewayRXInfo{
			{
				GatewayID: ts.Gateway.GatewayID,
				RSSI:      -50,
				LoRaSNR:   -3,
				Antenna:   2,
				Board:     1,
			},
		},
	}
	assert.NoError(storage.SaveDeviceGatewayRXInfoSet(context.Background(), deviceGatewayRXInfoSet))

	// add device-queue items
	assert.NoError(storage.FlushDeviceQueueForDevEUI(context.Background(), storage.DB(), tst.DeviceSession.DevEUI))
	for _, qi := range tst.DeviceQueueItems {
		assert.NoError(storage.CreateDeviceQueueItem(context.Background(), storage.DB(), &qi, *ts.DeviceProfile, tst.DeviceSession))
	}

	// run queue scheduler
	assert.NoError(downlink.ScheduleDeviceQueueBatch(context.Background(), 1))

	// refresh device-session
	var err error
	ds, err := storage.GetDeviceSession(context.Background(), ts.DeviceSession.DevEUI)
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
	assert.NoError(storage.UpdateMulticastGroup(context.Background(), storage.DB(), &tst.MulticastGroup))

	// add multicast queue items
	assert.NoError(storage.FlushMulticastQueueForMulticastGroup(context.Background(), storage.DB(), ts.MulticastGroup.ID))
	for _, qi := range tst.MulticastQueueItems {
		assert.NoError(storage.CreateMulticastQueueItem(context.Background(), storage.DB(), &qi))
	}

	// run multicast scheduler
	assert.NoError(downlink.ScheduleMulticastQueueBatch(context.Background(), 1))

	// run assertions
	for _, a := range tst.Assert {
		a(assert, ts)
	}
}

// AssertOTAATest asserts the given OTAA test.
func (ts *IntegrationTestSuite) AssertOTAATest(t *testing.T, tst OTAATest) {
	assert := require.New(t)

	storage.RedisClient().FlushAll()

	ts.FlushClients()
	ts.initConfig()

	if tst.BeforeFunc != nil {
		assert.NoError(tst.BeforeFunc(&tst))
	}

	for _, f := range tst.ExtraChannels {
		assert.NoError(band.Band().AddChannel(f, 0, 5))
	}

	// set mocks
	ts.backendAPIResponse = tst.JoinServerJoinAnsPayload

	// create device-activations
	assert.NoError(storage.DeleteDeviceActivationsForDevice(context.Background(), storage.DB(), ts.Device.DevEUI))
	for _, da := range tst.DeviceActivations {
		assert.NoError(storage.CreateDeviceActivation(context.Background(), storage.DB(), &da))
	}

	// create device-queue items
	assert.NoError(storage.FlushDeviceQueueForDevEUI(context.Background(), storage.DB(), ts.Device.DevEUI))
	for _, qi := range tst.DeviceQueueItems {
		assert.NoError(storage.CreateDeviceQueueItem(context.Background(), storage.DB(), &qi, *ts.DeviceProfile, storage.DeviceSession{}))
	}

	phyB, err := tst.PHYPayload.MarshalBinary()
	assert.NoError(err)

	err = uplink.HandleUplinkFrame(context.Background(), gw.UplinkFrame{
		RxInfo:     &tst.RXInfo,
		TxInfo:     &tst.TXInfo,
		PhyPayload: phyB,
	})
	if err != nil {
		if tst.ExpectedError == nil {
			assert.NoError(err)
		} else {
			assert.Equal(tst.ExpectedError.Error(), err.Error())
		}
	} else {
		assert.NoError(tst.ExpectedError)
	}

	// run assertions
	for _, a := range tst.Assert {
		a(assert, ts)
	}

	if tst.AfterFunc != nil {
		assert.NoError(tst.AfterFunc(&tst))
	}
}

// AssertRejoinTest asserts the given rejoin test.
func (ts *IntegrationTestSuite) AssertRejoinTest(t *testing.T, tst RejoinTest) {
	assert := require.New(t)

	storage.RedisClient().FlushAll()
	ts.FlushClients()

	if tst.BeforeFunc != nil {
		assert.NoError(tst.BeforeFunc(&tst))
	}

	// overwrite device-session
	ts.CreateDeviceSession(tst.DeviceSession)

	// set mocks
	ts.backendAPIResponse = tst.JoinServerRejoinAnsPayload

	phyB, err := tst.PHYPayload.MarshalBinary()
	assert.NoError(err)

	err = uplink.HandleUplinkFrame(context.Background(), gw.UplinkFrame{
		RxInfo:     &tst.RXInfo,
		TxInfo:     &tst.TXInfo,
		PhyPayload: phyB,
	})
	if err != nil {
		if tst.ExpectedError == nil {
			assert.NoError(err)
		} else {
			assert.Equal(tst.ExpectedError.Error(), err.Error())
		}
		return
	}
	assert.NoError(tst.ExpectedError)

	// run assertions
	for _, a := range tst.Assert {
		a(assert, ts)
	}

	if tst.AfterFunc != nil {
		assert.NoError(tst.AfterFunc(&tst))
	}
}

// AssertClassATest asserts the given class-a test.
func (ts *IntegrationTestSuite) AssertClassATest(t *testing.T, tst ClassATest) {
	assert := require.New(t)

	storage.RedisClient().FlushAll()

	if tst.BeforeFunc != nil {
		assert.NoError(tst.BeforeFunc(&tst))
	}

	// overwrite device-session
	ts.CreateDeviceSession(tst.DeviceSession)

	// add device-queue items
	assert.NoError(storage.FlushDeviceQueueForDevEUI(context.Background(), storage.DB(), tst.DeviceSession.DevEUI))
	for _, qi := range tst.DeviceQueueItems {
		assert.NoError(storage.CreateDeviceQueueItem(context.Background(), storage.DB(), &qi, *ts.DeviceProfile, tst.DeviceSession))
	}

	// set mac-command queue
	for _, qi := range tst.MACCommandQueueItems {
		assert.NoError(storage.CreateMACCommandQueueItem(context.Background(), ts.Device.DevEUI, qi))
	}

	// set pending mac-commands
	for _, pending := range tst.PendingMACCommands {
		assert.NoError(storage.SetPendingMACCommand(context.Background(), ts.Device.DevEUI, pending))
	}

	// set mocks
	ts.ASClient.HandleDataUpErr = tst.ASHandleUplinkDataError

	// tst.PHYPayload.SetUplinkDataMIC(lorawan.LoRaWAN1_0, 0, 0, 0, tst.DeviceSession.FNwkSIntKey, tst.DeviceSession.SNwkSIntKey)
	// fmt.Printf("\n\n%+v\n\n", tst.PHYPayload.MIC)

	phyB, err := tst.PHYPayload.MarshalBinary()
	assert.NoError(err)

	err = uplink.HandleUplinkFrame(context.Background(), gw.UplinkFrame{
		RxInfo:     &tst.RXInfo,
		TxInfo:     &tst.TXInfo,
		PhyPayload: phyB,
	})
	if err != nil {
		if tst.ExpectedError == nil {
			assert.NoError(err)
		} else {
			assert.Equal(tst.ExpectedError.Error(), err.Error())
		}
	} else {
		assert.NoError(tst.ExpectedError)
	}

	// refresh device-session
	ds, err := storage.GetDeviceSession(context.Background(), ts.DeviceSession.DevEUI)
	assert.NoError(err)
	ts.DeviceSession = &ds

	// run assertions
	for _, a := range tst.Assert {
		a(assert, ts)
	}

	if tst.AfterFunc != nil {
		assert.NoError(tst.AfterFunc(&tst))
	}
}

// AssertDownlinkProprietaryTest asserts the given downlink proprietary test.
func (ts *IntegrationTestSuite) AssertDownlinkProprietaryTest(t *testing.T, tst DownlinkProprietaryTest) {
	assert := require.New(t)

	_, err := ts.NSAPI.SendProprietaryPayload(context.Background(), &tst.SendProprietaryPayloadRequest)
	assert.NoError(err)

	// run assertions
	for _, a := range tst.Assert {
		a(assert, ts)
	}
}

// AssertUplinkProprietaryTest assers the given uplink proprietary test.
func (ts *IntegrationTestSuite) AssertUplinkProprietaryTest(t *testing.T, tst UplinkProprietaryTest) {
	assert := require.New(t)

	storage.RedisClient().FlushAll()
	ts.FlushClients()

	phyB, err := tst.PHYPayload.MarshalBinary()
	assert.NoError(err)

	assert.NoError(uplink.HandleUplinkFrame(context.Background(), gw.UplinkFrame{
		PhyPayload: phyB,
		TxInfo:     &tst.TXInfo,
		RxInfo:     &tst.RXInfo,
	}))

	// run assertions
	for _, a := range tst.Assert {
		a(assert, ts)
	}
}

// AssertDownlinkTXAckTest asserts the given downlink tx ack test.
func (ts *IntegrationTestSuite) AssertDownlinkTXAckTest(t *testing.T, tst DownlinkTXAckTest) {
	assert := require.New(t)
	storage.RedisClient().FlushAll()

	// set device-session
	assert.NoError(storage.SaveDeviceSession(context.Background(), tst.DeviceSession))

	// update multicast-group to reset frame-counter increment
	assert.NoError(storage.UpdateMulticastGroup(context.Background(), storage.DB(), ts.MulticastGroup))

	// flush & add device-queue items
	assert.NoError(storage.FlushDeviceQueueForDevEUI(context.Background(), storage.DB(), tst.DeviceSession.DevEUI))
	for i, qi := range tst.DeviceQueueItems {
		assert.NoError(storage.CreateDeviceQueueItem(context.Background(), storage.DB(), &qi, storage.DeviceProfile{}, storage.DeviceSession{}))
		if i == 0 {
			tst.DownlinkFrame.DeviceQueueItemId = qi.ID
		}
	}

	// flush & add multicast-queue items
	assert.NoError(storage.FlushMulticastQueueForMulticastGroup(context.Background(), storage.DB(), ts.MulticastGroup.ID))
	for i, qi := range tst.MulticastQueueItems {
		assert.NoError(storage.CreateMulticastQueueItem(context.Background(), storage.DB(), &qi))
		if i == 0 {
			tst.DownlinkFrame.MulticastQueueItemId = qi.ID
		}
	}

	// save DownlinkFrame object
	assert.NoError(storage.SaveDownlinkFrame(context.Background(), tst.DownlinkFrame))

	err := ack.HandleDownlinkTXAck(context.Background(), tst.DownlinkTXAck)
	if err != nil {
		if tst.ExpectedError == nil {
			assert.NoError(err)
		} else {
			assert.Equal(tst.ExpectedError.Error(), err.Error())
		}
		return
	}
	assert.NoError(tst.ExpectedError)

	// run assertions
	for _, a := range tst.Assert {
		a(assert, ts)
	}
}

func (ts *IntegrationTestSuite) initConfig() {
	conf := test.GetConfig()

	band.Setup(conf)
	uplink.Setup(conf)
	downlink.Setup(conf)

	storage.SetTimeLocation("Europe/Amsterdam")
}
