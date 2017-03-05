package session

import (
	"errors"
	"fmt"
	"testing"

	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/test"
	"github.com/brocaar/lorawan"
	. "github.com/smartystreets/goconvey/convey"
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
				if devAddr.NwkID() != netID.NwkID() {
					t.Fatalf("%b must equal %b", devAddr.NwkID(), netID.NwkID())
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
	Convey("Given an empty node-session", t, func() {
		ns := NodeSession{}

		Convey("When appending 30 items to the UplinkHistory", func() {
			for i := uint32(0); i < 30; i++ {
				ns.AppendUplinkHistory(UplinkHistory{FCnt: i})
			}

			Convey("Then only the last 20 items are preserved", func() {
				So(ns.UplinkHistory, ShouldHaveLength, 20)
				So(ns.UplinkHistory[19].FCnt, ShouldEqual, 29)
				So(ns.UplinkHistory[0].FCnt, ShouldEqual, 10)
			})
		})

		Convey("In case of adding the same FCnt twice", func() {
			ns.AppendUplinkHistory(UplinkHistory{FCnt: 10, MaxSNR: 5})
			ns.AppendUplinkHistory(UplinkHistory{FCnt: 10, MaxSNR: 6})

			Convey("Then the first record is kept", func() {
				So(ns.UplinkHistory, ShouldHaveLength, 1)
				So(ns.UplinkHistory[0].MaxSNR, ShouldEqual, 5)
			})
		})

		Convey("When appending 20 items, with two missing frames", func() {
			for i := uint32(0); i < 20; i++ {
				if i < 5 {
					ns.AppendUplinkHistory(UplinkHistory{FCnt: i})
					continue
				}

				if i < 10 {
					ns.AppendUplinkHistory(UplinkHistory{FCnt: i + 1})
					continue
				}

				ns.AppendUplinkHistory(UplinkHistory{FCnt: i + 2})
			}

			Convey("Then the packet-loss is 10%", func() {
				So(ns.GetPacketLossPercentage(), ShouldEqual, 10)
			})
		})
	})
}

func TestNodeSession(t *testing.T) {
	conf := test.GetConfig()

	Convey("Given a clean Redis database", t, func() {
		p := common.NewRedisPool(conf.RedisURL)
		test.MustFlushRedis(p)

		Convey("Given a NodeSession", func() {
			ns := NodeSession{
				DevAddr: [4]byte{1, 2, 3, 4},
				DevEUI:  [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
			}

			Convey("When getting a non-existing NodeSession", func() {
				_, err := GetNodeSession(p, ns.DevEUI)
				Convey("Then an error is returned", func() {
					So(err, ShouldResemble, errors.New("get node-session 0102030405060708 error: redigo: nil returned"))
				})
			})

			Convey("When checking if a non-existing NodeSession exists", func() {
				exists, err := NodeSessionExists(p, ns.DevEUI)
				So(err, ShouldBeNil)

				Convey("Then false is returned", func() {
					So(exists, ShouldBeFalse)
				})
			})

			Convey("When saving the NodeSession", func() {
				So(SaveNodeSession(p, ns), ShouldBeNil)

				Convey("Then when getting the NodeSessions for its DevAddr, it contains the NodeSession", func() {
					sessions, err := GetNodeSessionsForDevAddr(p, ns.DevAddr)
					So(err, ShouldBeNil)
					So(sessions, ShouldHaveLength, 1)
					So(sessions[0], ShouldResemble, ns)
				})

				Convey("Then the session can be retrieved by it's DevEUI", func() {
					ns2, err := GetNodeSession(p, ns.DevEUI)
					So(err, ShouldBeNil)
					So(ns2, ShouldResemble, ns)
				})

				Convey("When checking if a NodeSession exists", func() {
					exists, err := NodeSessionExists(p, ns.DevEUI)
					So(err, ShouldBeNil)

					Convey("Then true is returned", func() {
						So(exists, ShouldBeTrue)
					})
				})
			})

			Convey("When calling validateAndGetFullFCntUp", func() {
				testTable := []struct {
					ServerFCnt uint32
					NodeFCnt   uint32
					FullFCnt   uint32
					Valid      bool
				}{
					{0, 1, 1, true},                                                               // one packet was lost
					{1, 1, 1, true},                                                               // ideal case, the FCnt has the expected value
					{2, 1, 0, false},                                                              // old packet received or re-transmission
					{0, common.Band.MaxFCntGap, 0, false},                                         // gap should be less than MaxFCntGap
					{0, common.Band.MaxFCntGap - 1, common.Band.MaxFCntGap - 1, true},             // gap is exactly within the allowed MaxFCntGap
					{65536, common.Band.MaxFCntGap - 1, common.Band.MaxFCntGap - 1 + 65536, true}, // roll-over happened, gap ix exactly within allowed MaxFCntGap
					{65535, common.Band.MaxFCntGap, 0, false},                                     // roll-over happened, but too many lost frames
					{65535, 0, 65536, true},                                                       // roll-over happened
					{65536, 0, 65536, true},                                                       // re-transmission
					{4294967295, 0, 0, true},                                                      // 32 bit roll-over happened, counter started at 0 again
				}

				for _, test := range testTable {
					Convey(fmt.Sprintf("Then when FCntUp=%d, ValidateAndGetFullFCntUp(%d) should return (%d, %t)", test.ServerFCnt, test.NodeFCnt, test.FullFCnt, test.Valid), func() {
						ns.FCntUp = test.ServerFCnt
						fullFCntUp, ok := ValidateAndGetFullFCntUp(ns, test.NodeFCnt)
						So(ok, ShouldEqual, test.Valid)
						So(fullFCntUp, ShouldEqual, test.FullFCnt)
					})
				}
			})

		})
	})
}

func TestGetNodeSessionForPHYPayload(t *testing.T) {
	conf := test.GetConfig()

	Convey("Given a clean Redis database with a set of node-sessions for the same DevAddr", t, func() {
		p := common.NewRedisPool(conf.RedisURL)
		test.MustFlushRedis(p)

		devAddr := lorawan.DevAddr{1, 2, 3, 4}

		nodeSessions := []NodeSession{
			{
				DevAddr:   devAddr,
				DevEUI:    lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 1},
				NwkSKey:   lorawan.AES128Key{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
				FCntUp:    100,
				RelaxFCnt: true,
			},
			{
				DevAddr:   devAddr,
				DevEUI:    lorawan.EUI64{2, 2, 2, 2, 2, 2, 2, 2},
				NwkSKey:   lorawan.AES128Key{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2},
				FCntUp:    200,
				RelaxFCnt: false,
			},
		}
		for _, ns := range nodeSessions {
			So(SaveNodeSession(p, ns), ShouldBeNil)
		}

		Convey("Given a set of tests", func() {
			testTable := []struct {
				Name           string
				DevAddr        lorawan.DevAddr
				NwkSKey        lorawan.AES128Key
				FCnt           uint32
				ExpectedDevEUI lorawan.EUI64
				ExpectedFCntUp uint32
				ExpectedError  error
			}{
				{
					Name:           "matching DevEUI 0101010101010101",
					DevAddr:        devAddr,
					NwkSKey:        nodeSessions[0].NwkSKey,
					FCnt:           nodeSessions[0].FCntUp,
					ExpectedFCntUp: nodeSessions[0].FCntUp,
					ExpectedDevEUI: nodeSessions[0].DevEUI,
				},
				{
					Name:           "matching DevEUI 0202020202020202",
					DevAddr:        devAddr,
					NwkSKey:        nodeSessions[1].NwkSKey,
					FCnt:           nodeSessions[1].FCntUp,
					ExpectedFCntUp: nodeSessions[1].FCntUp,
					ExpectedDevEUI: nodeSessions[1].DevEUI,
				},
				{
					Name:           "matching DevEUI 0101010101010101 with frame counter reset",
					DevAddr:        devAddr,
					NwkSKey:        nodeSessions[0].NwkSKey,
					FCnt:           0,
					ExpectedFCntUp: 0, // has been reset
					ExpectedDevEUI: nodeSessions[0].DevEUI,
				},
				{
					Name:          "matching DevEUI 0202020202020202 with invalid frame counter",
					DevAddr:       devAddr,
					NwkSKey:       nodeSessions[1].NwkSKey,
					FCnt:          0,
					ExpectedError: errors.New("node-session does not exist or invalid fcnt or mic"),
				},
				{
					Name:          "invalid DevAddr",
					DevAddr:       lorawan.DevAddr{1, 1, 1, 1},
					NwkSKey:       nodeSessions[0].NwkSKey,
					FCnt:          nodeSessions[0].FCntUp,
					ExpectedError: errors.New("node-session does not exist or invalid fcnt or mic"),
				},
				{
					Name:          "invalid NwkSKey",
					DevAddr:       nodeSessions[0].DevAddr,
					NwkSKey:       lorawan.AES128Key{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
					FCnt:          nodeSessions[0].FCntUp,
					ExpectedError: errors.New("node-session does not exist or invalid fcnt or mic"),
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
					So(phy.SetMIC(test.NwkSKey), ShouldBeNil)

					ns, err := GetNodeSessionForPHYPayload(p, phy)
					So(err, ShouldResemble, test.ExpectedError)
					if test.ExpectedError != nil {
						return
					}

					// "refresh" the ns, to test if the FCnt has been updated
					// in case of a frame counter reset
					ns, err = GetNodeSession(p, ns.DevEUI)
					So(err, ShouldBeNil)

					So(ns.DevEUI, ShouldResemble, test.ExpectedDevEUI)
					So(ns.FCntUp, ShouldEqual, test.ExpectedFCntUp)
				})
			}
		})
	})
}
