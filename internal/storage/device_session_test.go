package storage

import (
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/test"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/band"
)

func TestGetRandomDevAddr(t *testing.T) {
	conf := test.GetConfig()

	Convey("Given a Redis database and NetID 010203", t, func() {
		p := common.NewRedisPool(conf.RedisURL)
		test.MustFlushRedis(p)
		netID := lorawan.NetID{1, 2, 3}

		Convey("When calling getRandomDevAddr many times, it should always return an unique DevAddr", func() {
			log := make(map[lorawan.DevAddr]struct{})
			for i := 0; i < 1000; i++ {
				devAddr, err := GetRandomDevAddr(p, netID)
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

func TestDeviceSession(t *testing.T) {
	conf := test.GetConfig()

	Convey("Given a clean Redis database", t, func() {
		p := common.NewRedisPool(conf.RedisURL)
		test.MustFlushRedis(p)

		Convey("Given a device-session", func() {
			s := DeviceSession{
				DevAddr:              lorawan.DevAddr{1, 2, 3, 4},
				DevEUI:               lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
				ExtraUplinkChannels:  map[int]band.Channel{},
				UplinkGatewayHistory: map[lorawan.EUI64]UplinkGatewayHistory{},
				RX2Frequency:         869525000,
			}

			Convey("When getting a non-existing device-session", func() {
				_, err := GetDeviceSession(p, s.DevEUI)

				Convey("Then the expected error is returned", func() {
					So(err, ShouldResemble, ErrDoesNotExist)
				})
			})

			Convey("When saving the device-session", func() {
				So(SaveDeviceSession(p, s), ShouldBeNil)

				Convey("Then GetDeviceSessionsForDevAddr includes the device-session", func() {
					sessions, err := GetDeviceSessionsForDevAddr(p, s.DevAddr)
					So(err, ShouldBeNil)
					So(sessions, ShouldHaveLength, 1)
					So(sessions[0], ShouldResemble, s)
				})

				Convey("Then the session can be retrieved by it's DevEUI", func() {
					s2, err := GetDeviceSession(p, s.DevEUI)
					So(err, ShouldBeNil)
					So(s2, ShouldResemble, s)
				})

				Convey("Then DeleteDeviceSession deletes the device-session", func() {
					So(DeleteDeviceSession(p, s.DevEUI), ShouldBeNil)
					So(DeleteDeviceSession(p, s.DevEUI), ShouldEqual, ErrDoesNotExist)

				})
			})

			Convey("When calling validateAndGetFullFCntUp", func() {
				defaults := config.C.NetworkServer.Band.Band.GetDefaults()

				testTable := []struct {
					ServerFCnt uint32
					NodeFCnt   uint32
					FullFCnt   uint32
					Valid      bool
				}{
					{0, 1, 1, true},                                                         // one packet was lost
					{1, 1, 1, true},                                                         // ideal case, the FCnt has the expected value
					{2, 1, 0, false},                                                        // old packet received or re-transmission
					{0, defaults.MaxFCntGap, 0, false},                                      // gap should be less than MaxFCntGap
					{0, defaults.MaxFCntGap - 1, defaults.MaxFCntGap - 1, true},             // gap is exactly within the allowed MaxFCntGap
					{65536, defaults.MaxFCntGap - 1, defaults.MaxFCntGap - 1 + 65536, true}, // roll-over happened, gap ix exactly within allowed MaxFCntGap
					{65535, defaults.MaxFCntGap, 0, false},                                  // roll-over happened, but too many lost frames
					{65535, 0, 65536, true},                                                 // roll-over happened
					{65536, 0, 65536, true},                                                 // re-transmission
					{4294967295, 0, 0, true},                                                // 32 bit roll-over happened, counter started at 0 again
				}

				for _, test := range testTable {
					Convey(fmt.Sprintf("Then when FCntUp=%d, ValidateAndGetFullFCntUp(%d) should return (%d, %t)", test.ServerFCnt, test.NodeFCnt, test.FullFCnt, test.Valid), func() {
						s.FCntUp = test.ServerFCnt
						fullFCntUp, ok := ValidateAndGetFullFCntUp(s, test.NodeFCnt)
						So(ok, ShouldEqual, test.Valid)
						So(fullFCntUp, ShouldEqual, test.FullFCnt)
					})
				}
			})

		})
	})
}

func TestGetDeviceSessionForPHYPayload(t *testing.T) {
	conf := test.GetConfig()

	Convey("Given a clean Redis database with a set of device-sessions for the same DevAddr", t, func() {
		p := common.NewRedisPool(conf.RedisURL)
		test.MustFlushRedis(p)

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
			So(SaveDeviceSession(p, s), ShouldBeNil)
		}

		Convey("Given a set of tests", func() {
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
					ExpectedError: ErrDoesNotExistOrFCntOrMICInvalid,
				},
				{
					Name:          "invalid DevAddr",
					DevAddr:       lorawan.DevAddr{1, 1, 1, 1},
					FNwkSIntKey:   deviceSessions[0].FNwkSIntKey,
					SNwkSIntKey:   deviceSessions[0].SNwkSIntKey,
					FCnt:          deviceSessions[0].FCntUp,
					ExpectedError: ErrDoesNotExistOrFCntOrMICInvalid,
				},
				{
					Name:          "invalid NwkSKey",
					DevAddr:       deviceSessions[0].DevAddr,
					FNwkSIntKey:   lorawan.AES128Key{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
					SNwkSIntKey:   lorawan.AES128Key{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
					FCnt:          deviceSessions[0].FCntUp,
					ExpectedError: ErrDoesNotExistOrFCntOrMICInvalid,
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

			for i, test := range testTable {
				Convey(fmt.Sprintf("Testing: %s [%d]", test.Name, i), func() {
					phy := lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							MType: lorawan.UnconfirmedDataUp,
							Major: lorawan.LoRaWANR1,
						},
						MACPayload: &lorawan.MACPayload{
							FHDR: lorawan.FHDR{
								DevAddr: test.DevAddr,
								FCtrl:   lorawan.FCtrl{},
								FCnt:    test.FCnt,
							},
						},
					}
					So(phy.SetUplinkDataMIC(lorawan.LoRaWAN1_0, 0, 0, 0, test.FNwkSIntKey, test.SNwkSIntKey), ShouldBeNil)

					s, err := GetDeviceSessionForPHYPayload(p, phy, 0, 0)
					if test.ExpectedError != nil {
						So(err, ShouldNotBeNil)
						So(err.Error(), ShouldEqual, test.ExpectedError.Error())
						return
					}
					So(err, ShouldBeNil)
					So(s.DevEUI, ShouldResemble, test.ExpectedDevEUI)
					So(s.FCntUp, ShouldEqual, test.ExpectedFCntUp)
				})
			}
		})
	})
}
