package storage

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"

	"github.com/brocaar/chirpstack-network-server/internal/test"
	"github.com/brocaar/lorawan"
	loraband "github.com/brocaar/lorawan/band"
)

func TestGetRandomDevAddr(t *testing.T) {
	conf := test.GetConfig()
	if err := Setup(conf); err != nil {
		t.Fatal(err)
	}

	Convey("Given a Redis database and NetID 010203", t, func() {
		RedisClient().FlushAll()
		netID := lorawan.NetID{1, 2, 3}

		Convey("When calling getRandomDevAddr many times, it should always return an unique DevAddr", func() {
			log := make(map[lorawan.DevAddr]struct{})
			for i := 0; i < 1000; i++ {
				devAddr, err := GetRandomDevAddr(netID)
				if err != nil {
					t.Fatal(err)
				}
				if !devAddr.IsNetID(netID) {
					t.Fatalf("DevAddr %s does not have NetID %s prefix", devAddr, netID)
				}
				if len(log) != i {
					t.Fatalf("%d must equal %d", len(log), i)
				}
				log[devAddr] = struct{}{}
			}
		})
	})
}

func TestUplinkHistory(t *testing.T) {
	Convey("Given an empty device-session", t, func() {
		s := DeviceSession{}

		Convey("When appending 30 items to the UplinkHistory", func() {
			for i := uint32(0); i < 30; i++ {
				s.AppendUplinkHistory(UplinkHistory{FCnt: i})
			}

			Convey("Then only the last 20 items are preserved", func() {
				So(s.UplinkHistory, ShouldHaveLength, 20)
				So(s.UplinkHistory[19].FCnt, ShouldEqual, 29)
				So(s.UplinkHistory[0].FCnt, ShouldEqual, 10)
			})
		})

		Convey("In case of adding the same FCnt twice", func() {
			s.AppendUplinkHistory(UplinkHistory{FCnt: 10, MaxSNR: 5})
			s.AppendUplinkHistory(UplinkHistory{FCnt: 10, MaxSNR: 6})

			Convey("Then the first record is kept", func() {
				So(s.UplinkHistory, ShouldHaveLength, 1)
				So(s.UplinkHistory[0].MaxSNR, ShouldEqual, 5)
			})
		})

		Convey("When appending 20 items, with two missing frames", func() {
			for i := uint32(0); i < 20; i++ {
				if i < 5 {
					s.AppendUplinkHistory(UplinkHistory{FCnt: i})
					continue
				}

				if i < 10 {
					s.AppendUplinkHistory(UplinkHistory{FCnt: i + 1})
					continue
				}

				s.AppendUplinkHistory(UplinkHistory{FCnt: i + 2})
			}

			Convey("Then the packet-loss is 10%", func() {
				So(s.GetPacketLossPercentage(), ShouldEqual, 10)
			})
		})
	})
}

func (ts *StorageTestSuite) TestDeviceGatewayRXInfoSet() {
	devEUI := lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}

	ts.T().Run("Does not exist", func(t *testing.T) {
		assert := require.New(t)

		_, err := GetDeviceGatewayRXInfoSet(context.Background(), devEUI)
		assert.Equal(ErrDoesNotExist, err)

		sets, err := GetDeviceGatewayRXInfoSetForDevEUIs(context.Background(), []lorawan.EUI64{devEUI})
		assert.NoError(err)
		assert.Len(sets, 0)
	})

	ts.T().Run("Create", func(t *testing.T) {
		assert := require.New(t)

		rxInfoSet := DeviceGatewayRXInfoSet{
			DevEUI: devEUI,
			DR:     3,
			Items: []DeviceGatewayRXInfo{
				{
					GatewayID: lorawan.EUI64{2, 2, 3, 4, 5, 6, 7, 8},
					RSSI:      -60,
					LoRaSNR:   5.5,
					Board:     2,
					Antenna:   3,
					Context:   []byte{0x01, 0x02, 0x03, 0x04},
				},
			},
		}
		assert.NoError(SaveDeviceGatewayRXInfoSet(context.Background(), rxInfoSet))

		t.Run("Get", func(t *testing.T) {
			assert := require.New(t)

			rxInfoSetGet, err := GetDeviceGatewayRXInfoSet(context.Background(), devEUI)
			assert.NoError(err)
			assert.Equal(rxInfoSet, rxInfoSetGet)

			rxInfoSets, err := GetDeviceGatewayRXInfoSetForDevEUIs(context.Background(), []lorawan.EUI64{devEUI})
			assert.NoError(err)
			assert.Len(rxInfoSets, 1)
			assert.Equal(rxInfoSet, rxInfoSets[0])
		})

		t.Run("Delete", func(t *testing.T) {
			assert := require.New(t)

			assert.NoError(DeleteDeviceGatewayRXInfoSet(context.Background(), devEUI))
			_, err := GetDeviceGatewayRXInfoSet(context.Background(), devEUI)
			assert.Equal(ErrDoesNotExist, err)
			assert.Equal(ErrDoesNotExist, DeleteDeviceGatewayRXInfoSet(context.Background(), devEUI))
		})
	})
}

func (ts *StorageTestSuite) TestDeviceSession() {
	s := DeviceSession{
		DevAddr:              lorawan.DevAddr{1, 2, 3, 4},
		DevEUI:               lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
		ExtraUplinkChannels:  map[int]loraband.Channel{},
		RX2Frequency:         869525000,
		MACCommandErrorCount: make(map[lorawan.CID]int),
	}

	ts.T().Run("Get non existing", func(t *testing.T) {
		assert := require.New(t)
		_, err := GetDeviceSession(context.Background(), s.DevEUI)
		assert.Equal(ErrDoesNotExist, err)
	})

	ts.T().Run("Save", func(t *testing.T) {
		assert := require.New(t)
		assert.NoError(SaveDeviceSession(context.Background(), s))

		t.Run("Get", func(t *testing.T) {
			assert := require.New(t)
			s2, err := GetDeviceSession(context.Background(), s.DevEUI)
			assert.NoError(err)
			assert.Equal(s, s2)
		})

		t.Run("Delete", func(t *testing.T) {
			assert := require.New(t)
			assert.NoError(DeleteDeviceSession(context.Background(), s.DevEUI))
			assert.Equal(DeleteDeviceSession(context.Background(), s.DevEUI), ErrDoesNotExist)
		})
	})
}

func (ts *StorageTestSuite) TestGetDeviceSessionForPHYPayload() {
	assert := require.New(ts.T())
	devAddr := lorawan.DevAddr{1, 2, 3, 4}

	deviceSessions := []DeviceSession{
		{
			DevAddr:            devAddr,
			DevEUI:             lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 1},
			SNwkSIntKey:        lorawan.AES128Key{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
			FNwkSIntKey:        lorawan.AES128Key{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
			NwkSEncKey:         lorawan.AES128Key{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
			FCntUp:             100,
			SkipFCntValidation: true,
		},
		{
			DevAddr:            devAddr,
			DevEUI:             lorawan.EUI64{2, 2, 2, 2, 2, 2, 2, 2},
			SNwkSIntKey:        lorawan.AES128Key{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2},
			FNwkSIntKey:        lorawan.AES128Key{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2},
			NwkSEncKey:         lorawan.AES128Key{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2},
			FCntUp:             200,
			SkipFCntValidation: false,
		},
		{
			DevAddr:            devAddr,
			DevEUI:             lorawan.EUI64{3, 3, 3, 3, 3, 3, 3, 3},
			SNwkSIntKey:        lorawan.AES128Key{3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3},
			FNwkSIntKey:        lorawan.AES128Key{3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3},
			NwkSEncKey:         lorawan.AES128Key{3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3},
			FCntUp:             300,
			SkipFCntValidation: false,
			PendingRejoinDeviceSession: &DeviceSession{
				DevAddr:     lorawan.DevAddr{4, 3, 2, 1},
				DevEUI:      lorawan.EUI64{3, 3, 3, 3, 3, 3, 3, 3},
				SNwkSIntKey: lorawan.AES128Key{4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4},
				FNwkSIntKey: lorawan.AES128Key{4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4},
				NwkSEncKey:  lorawan.AES128Key{4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4},
				FCntUp:      0,
			},
		},
	}
	for _, s := range deviceSessions {
		assert.NoError(SaveDeviceSession(context.Background(), s))
	}

	testTable := []struct {
		Name           string
		DevAddr        lorawan.DevAddr
		SNwkSIntKey    lorawan.AES128Key
		FNwkSIntKey    lorawan.AES128Key
		FCnt           uint32
		ExpectedDevEUI lorawan.EUI64
		ExpectedFCntUp uint32
		ExpectedError  error
	}{
		{
			Name:           "matching DevEUI 0101010101010101",
			DevAddr:        devAddr,
			FNwkSIntKey:    deviceSessions[0].FNwkSIntKey,
			SNwkSIntKey:    deviceSessions[0].SNwkSIntKey,
			FCnt:           deviceSessions[0].FCntUp,
			ExpectedFCntUp: deviceSessions[0].FCntUp,
			ExpectedDevEUI: deviceSessions[0].DevEUI,
		},
		{
			Name:           "matching DevEUI 0202020202020202",
			DevAddr:        devAddr,
			FNwkSIntKey:    deviceSessions[1].FNwkSIntKey,
			SNwkSIntKey:    deviceSessions[1].SNwkSIntKey,
			FCnt:           deviceSessions[1].FCntUp,
			ExpectedFCntUp: deviceSessions[1].FCntUp,
			ExpectedDevEUI: deviceSessions[1].DevEUI,
		},
		{
			Name:           "matching DevEUI 0101010101010101 with frame counter reset",
			DevAddr:        devAddr,
			FNwkSIntKey:    deviceSessions[0].FNwkSIntKey,
			SNwkSIntKey:    deviceSessions[0].SNwkSIntKey,
			FCnt:           0,
			ExpectedFCntUp: 0, // has been reset
			ExpectedDevEUI: deviceSessions[0].DevEUI,
		},
		{
			Name:          "matching DevEUI 0202020202020202 with invalid frame counter",
			DevAddr:       devAddr,
			FNwkSIntKey:   deviceSessions[1].FNwkSIntKey,
			SNwkSIntKey:   deviceSessions[1].SNwkSIntKey,
			FCnt:          0,
			ExpectedError: ErrFrameCounterReset,
		},
		{
			Name:          "invalid DevAddr",
			DevAddr:       lorawan.DevAddr{1, 1, 1, 1},
			FNwkSIntKey:   deviceSessions[0].FNwkSIntKey,
			SNwkSIntKey:   deviceSessions[0].SNwkSIntKey,
			FCnt:          deviceSessions[0].FCntUp,
			ExpectedError: ErrDoesNotExist,
		},
		{
			Name:          "invalid NwkSKey",
			DevAddr:       deviceSessions[0].DevAddr,
			FNwkSIntKey:   lorawan.AES128Key{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			SNwkSIntKey:   lorawan.AES128Key{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
			FCnt:          deviceSessions[0].FCntUp,
			ExpectedError: ErrInvalidMIC,
		},
		{
			Name:           "matching pending rejoin device-session",
			DevAddr:        lorawan.DevAddr{4, 3, 2, 1},
			SNwkSIntKey:    lorawan.AES128Key{4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4},
			FNwkSIntKey:    lorawan.AES128Key{4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4},
			FCnt:           0,
			ExpectedDevEUI: lorawan.EUI64{3, 3, 3, 3, 3, 3, 3, 3},
			ExpectedFCntUp: 0,
		},
	}

	for _, tst := range testTable {
		ts.T().Run(tst.Name, func(t *testing.T) {
			assert := require.New(t)

			phy := lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: tst.DevAddr,
						FCtrl:   lorawan.FCtrl{},
						FCnt:    tst.FCnt,
					},
				},
			}
			assert.NoError(phy.SetUplinkDataMIC(lorawan.LoRaWAN1_0, 0, 0, 0, tst.FNwkSIntKey, tst.SNwkSIntKey))

			s, err := GetDeviceSessionForPHYPayload(context.Background(), phy, 0, 0)
			if tst.ExpectedError != nil {
				assert.NotNil(err)
				assert.Equal(tst.ExpectedError.Error(), err.Error())
				return
			}

			assert.NoError(err)
			assert.Equal(tst.ExpectedDevEUI, s.DevEUI)
			assert.Equal(tst.ExpectedFCntUp, s.FCntUp)
		})
	}
}

func TestGetFullFCntUp(t *testing.T) {
	assert := require.New(t)

	// note that the ServerFCnt stores the expected frame-counter for the next
	// uplink, so in the ideal case the ServerFCnt and DeviceFCnt are equal.
	tests := []struct {
		ServerFCnt uint32
		DeviceFCnt uint32
		FullFCnt   uint32
	}{
		{1, 1, 1},                                 // frame-counter is as expected
		{1 << 16, 0, 1 << 16},                     // frame-counter is as expected
		{(1 << 16) + 1, 1, (1 << 16) + 1},         // frame-counter is as expected
		{0, 1, 1},                                 // one frame packet-loss
		{(1 << 16) + 1, 2, (1 << 16) + 2},         // one frame packet-loss
		{2, 1, 1},                                 // re-transmission of previous frame
		{(1 << 16) + 1, 0, (1 << 16)},             // re-transmission of previous frame
		{(1 << 16), (1 << 16) - 1, (1 << 16) - 1}, // re-transmission of previous frame
		{(1 << 32) - 1, 0, 0},                     // 32bit frame-counter rollover
	}

	for i, test := range tests {
		out := GetFullFCntUp(DeviceSession{FCntUp: test.ServerFCnt}, test.DeviceFCnt)
		assert.Equalf(test.FullFCnt, out, "Test %d: expected %d, got %d", i, test.FullFCnt, out)
	}
}
