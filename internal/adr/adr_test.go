package adr

import (
	"fmt"
	"testing"

	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/maccommand"
	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/session"
	"github.com/brocaar/loraserver/internal/test"
	"github.com/brocaar/lorawan"
	. "github.com/smartystreets/goconvey/convey"
)

func TestADR(t *testing.T) {
	conf := test.GetConfig()

	Convey("Testing the ADR functions", t, func() {
		Convey("Given a testtable for getNbRep", func() {
			testTable := []struct {
				PktLossRate   float64
				CurrentNbRep  uint8
				ExpectedNbRep uint8
			}{
				{4.99, 3, 2},
				{9.99, 2, 2},
				{29.99, 1, 2},
				{30, 3, 3},
			}

			for i, tst := range testTable {
				Convey(fmt.Sprintf("Given PktLossRate: %f, Current NbRep: %d [%d]", tst.PktLossRate, tst.CurrentNbRep, i), func() {
					Convey(fmt.Sprintf("Then NbRep equals: %d", tst.ExpectedNbRep), func() {
						So(getNbRep(tst.CurrentNbRep, tst.PktLossRate), ShouldEqual, tst.ExpectedNbRep)
					})
				})
			}
		})

		Convey("Given a node-session with TXPower: 0", func() {
			ns := session.NodeSession{}
			Convey("Then getCurrentTXPower returns the DefaultTXPower", func() {
				So(getCurrentTXPower(&ns), ShouldEqual, common.Band.DefaultTXPower)
			})
		})

		Convey("Given a node-session with TXPower set", func() {
			ns := session.NodeSession{TXPower: 11}
			Convey("Then getCurrentTXPower returns the same TXPower", func() {
				So(getCurrentTXPower(&ns), ShouldEqual, ns.TXPower)
			})
		})

		Convey("getMaxTXPower returns 20", func() {
			So(getMaxTXPower(), ShouldEqual, 20)
		})

		Convey("getMinTXPower returns 2", func() {
			So(getMinTXPower(), ShouldEqual, 2)
		})

		Convey("Given a testtable for getTXPowerIndex", func() {
			testTable := []struct {
				TXPower       int
				ExpectedIndex int
			}{
				{21, 0},
				{20, 0},
				{19, 0},
				{15, 0},
				{14, 1},
				{12, 1},
				{11, 2},
				{9, 2},
				{8, 3},
				{6, 3},
				{5, 4},
				{3, 4},
				{2, 5},
				{1, 5},
			}

			for i, tst := range testTable {
				Convey(fmt.Sprintf("Given TXPower is %d, then the returned TXPower index is: %d [%d]", tst.TXPower, tst.ExpectedIndex, i), func() {
					So(getTXPowerIndex(tst.TXPower), ShouldEqual, tst.ExpectedIndex)
				})
			}
		})

		Convey("Given a testtable for getIdealTXPowerAndDR", func() {
			testTable := []struct {
				NStep           int
				TXPower         int
				DR              int
				ExpectedTXPower int
				ExpectedDR      int
			}{
				{NStep: 0, TXPower: 14, DR: 3, ExpectedTXPower: 14, ExpectedDR: 3},
				{NStep: 1, TXPower: 14, DR: 4, ExpectedTXPower: 14, ExpectedDR: 5},
				{NStep: 1, TXPower: 14, DR: 5, ExpectedTXPower: 11, ExpectedDR: 5},
				{NStep: 2, TXPower: 14, DR: 4, ExpectedTXPower: 11, ExpectedDR: 5},
				{NStep: -1, TXPower: 14, DR: 4, ExpectedTXPower: 17, ExpectedDR: 4},
				{NStep: -1, TXPower: 20, DR: 4, ExpectedTXPower: 20, ExpectedDR: 4},
			}

			for i, tst := range testTable {
				Convey(fmt.Sprintf("Given NStep: %d, TXPower: %d, DR: %d [%d]", tst.NStep, tst.TXPower, tst.DR, i), func() {
					Convey(fmt.Sprintf("Then the ideal TXPower is %d and DR %d", tst.ExpectedTXPower, tst.ExpectedDR), func() {
						idealTXPower, idealDR := getIdealTXPowerAndDR(tst.NStep, tst.TXPower, tst.DR)
						So(idealTXPower, ShouldEqual, tst.ExpectedTXPower)
						So(idealDR, ShouldEqual, tst.ExpectedDR)
					})
				})
			}
		})

		Convey("Given a clean Redis database", func() {
			p := common.NewRedisPool(conf.RedisURL)
			test.MustFlushRedis(p)

			ctx := common.Context{
				RedisPool: p,
			}

			Convey("Given a testtable for HandleADR", func() {
				phyPayloadNoADR := lorawan.PHYPayload{
					MACPayload: &lorawan.MACPayload{
						FHDR: lorawan.FHDR{
							FCtrl: lorawan.FCtrl{
								ADR: false,
							},
						},
					},
				}

				phyPayloadADR := lorawan.PHYPayload{
					MACPayload: &lorawan.MACPayload{
						FHDR: lorawan.FHDR{
							FCtrl: lorawan.FCtrl{
								ADR: true,
							},
						},
					},
				}

				macCommand := lorawan.MACCommand{
					CID: lorawan.LinkADRReq,
					Payload: &lorawan.LinkADRReqPayload{
						DataRate: 3,
						TXPower:  1,                                // 14
						ChMask:   lorawan.ChMask{true, true, true}, // ADR applies to first three standard channels
						Redundancy: lorawan.Redundancy{
							ChMaskCntl: 0, // first block of 16 channels
							NbRep:      1,
						},
					},
				}
				macCommandB, err := macCommand.MarshalBinary()
				So(err, ShouldBeNil)

				macCommandCFList := lorawan.MACCommand{
					CID: lorawan.LinkADRReq,
					Payload: &lorawan.LinkADRReqPayload{
						DataRate: 3,
						TXPower:  1, // 14
						ChMask:   lorawan.ChMask{true, true, true, true, true, false, true},
						Redundancy: lorawan.Redundancy{
							ChMaskCntl: 0, // first block of 16 channels
							NbRep:      1,
						},
					},
				}
				macCommandCFListB, err := macCommandCFList.MarshalBinary()
				So(err, ShouldBeNil)

				testTable := []struct {
					Name                    string
					NodeSession             *session.NodeSession
					RXPacket                models.RXPacket
					FullFCnt                uint32
					ExpectedNodeSession     session.NodeSession
					ExpectedMACPayloadQueue []maccommand.QueueItem
					ExpectedError           error
				}{
					{
						Name: "ADR increasing data-rate by one step (no CFlist)",
						NodeSession: &session.NodeSession{
							DevAddr:            [4]byte{1, 2, 3, 4},
							DevEUI:             [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
							ADRInterval:        1,
							InstallationMargin: 5,
						},
						RXPacket: models.RXPacket{
							PHYPayload: phyPayloadADR,
							RXInfoSet: models.RXInfoSet{
								{DataRate: common.Band.DataRates[2], LoRaSNR: -7},
							},
						},
						FullFCnt: 1,
						ExpectedNodeSession: session.NodeSession{
							DevAddr:            [4]byte{1, 2, 3, 4},
							DevEUI:             [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
							ADRInterval:        1,
							InstallationMargin: 5,
							TXPower:            14,
							NbTrans:            1,
							UplinkHistory: []session.UplinkHistory{
								{FCnt: 1, MaxSNR: -7, GatewayCount: 1},
							},
						},
						ExpectedMACPayloadQueue: []maccommand.QueueItem{
							{DevEUI: [8]byte{1, 2, 3, 4, 5, 6, 7, 8}, Data: macCommandB},
						},
						ExpectedError: nil,
					},
					{
						Name: "ADR increasing data-rate by one step (with CFlist)",
						NodeSession: &session.NodeSession{
							DevAddr:            [4]byte{1, 2, 3, 4},
							DevEUI:             [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
							ADRInterval:        1,
							InstallationMargin: 5,
							CFList: &lorawan.CFList{
								868400000,
								868500000,
								0,
								868600000,
							},
						},
						RXPacket: models.RXPacket{
							PHYPayload: phyPayloadADR,
							RXInfoSet: models.RXInfoSet{
								{DataRate: common.Band.DataRates[2], LoRaSNR: -7},
							},
						},
						FullFCnt: 1,
						ExpectedNodeSession: session.NodeSession{
							DevAddr:            [4]byte{1, 2, 3, 4},
							DevEUI:             [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
							ADRInterval:        1,
							InstallationMargin: 5,
							CFList: &lorawan.CFList{
								868400000,
								868500000,
								0,
								868600000,
							},
							TXPower: 14,
							NbTrans: 1,
							UplinkHistory: []session.UplinkHistory{
								{FCnt: 1, MaxSNR: -7, GatewayCount: 1},
							},
						},
						ExpectedMACPayloadQueue: []maccommand.QueueItem{
							{DevEUI: [8]byte{1, 2, 3, 4, 5, 6, 7, 8}, Data: macCommandCFListB},
						},
						ExpectedError: nil,
					},
					{
						Name: "data-rate can be increased, but no ADR flag set",
						NodeSession: &session.NodeSession{
							DevAddr:            [4]byte{1, 2, 3, 4},
							DevEUI:             [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
							ADRInterval:        1,
							InstallationMargin: 5,
						},
						RXPacket: models.RXPacket{
							PHYPayload: phyPayloadNoADR,
							RXInfoSet: models.RXInfoSet{
								{DataRate: common.Band.DataRates[2], LoRaSNR: -7},
							},
						},
						FullFCnt: 1,
						ExpectedNodeSession: session.NodeSession{
							DevAddr:            [4]byte{1, 2, 3, 4},
							DevEUI:             [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
							ADRInterval:        1,
							InstallationMargin: 5,
							UplinkHistory: []session.UplinkHistory{
								{FCnt: 1, MaxSNR: -7, GatewayCount: 1},
							},
						},
						ExpectedError: nil,
					},
				}

				for i, tst := range testTable {
					Convey(fmt.Sprintf("Test: %s [%d]", tst.Name, i), func() {
						So(session.CreateNodeSession(p, *tst.NodeSession), ShouldBeNil)

						err := HandleADR(ctx, tst.NodeSession, tst.RXPacket, tst.FullFCnt)
						if tst.ExpectedError != nil {
							So(err, ShouldResemble, tst.ExpectedError)
							return
						}

						So(err, ShouldBeNil)
						So(tst.NodeSession, ShouldResemble, &tst.ExpectedNodeSession)

						macPayloadQueue, err := maccommand.ReadQueue(p, tst.NodeSession.DevAddr)
						So(err, ShouldBeNil)
						So(macPayloadQueue, ShouldResemble, tst.ExpectedMACPayloadQueue)
					})
				}
			})
		})
	})
}
