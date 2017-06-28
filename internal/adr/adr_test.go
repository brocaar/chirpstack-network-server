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

		Convey("Given a node-session with TXPowerIndex: 20 (out of range)", func() {
			ns := session.NodeSession{TXPowerIndex: 20}
			Convey("Then getCurrentTXPowerOffset returns 0", func() {
				So(getCurrentTXPowerOffset(&ns), ShouldEqual, 0)
			})
		})

		Convey("Given a node-session with TXPowerIndex set", func() {
			ns := session.NodeSession{TXPowerIndex: 1}
			Convey("Then getCurrentTXPowerOffset returns the expected value", func() {
				So(getCurrentTXPowerOffset(&ns), ShouldEqual, common.Band.TXPowerOffset[1])
			})
		})

		Convey("getMaxTXPowerOffset returns -14", func() {
			So(getMaxTXPowerOffset(), ShouldEqual, -14)
		})

		Convey("Given a testtable for getTXPowerIndexForOffset", func() {
			testTable := []struct {
				TXPowerOffset int
				ExpectedIndex int
			}{
				{0, 0},
				{-1, 0},
				{-2, 1},
				{-3, 1},
				{-4, 2},
				{-5, 2},
				{-6, 3},
				{-7, 3},
				{-8, 4},
				{-9, 4},
				{-10, 5},
				{-11, 5},
				{-12, 6},
				{-13, 6},
				{-14, 7},
				{-15, 7},
			}

			for i, tst := range testTable {
				Convey(fmt.Sprintf("Given TXPowerOffset is %d, then the returned TXPower index is: %d [%d]", tst.TXPowerOffset, tst.ExpectedIndex, i), func() {
					So(getTXPowerIndexForOffset(tst.TXPowerOffset), ShouldEqual, tst.ExpectedIndex)
				})
			}
		})

		Convey("Given a testtable for getIdealTXPowerAndDR", func() {
			testTable := []struct {
				NStep                 int
				TXPowerOffset         int
				DR                    int
				ExpectedTXPowerOffset int
				ExpectedDR            int
			}{
				{NStep: 0, TXPowerOffset: -2, DR: 3, ExpectedTXPowerOffset: -2, ExpectedDR: 3},
				{NStep: 1, TXPowerOffset: -2, DR: 4, ExpectedTXPowerOffset: -2, ExpectedDR: 5},
				{NStep: 1, TXPowerOffset: -2, DR: 5, ExpectedTXPowerOffset: -5, ExpectedDR: 5},
				{NStep: 2, TXPowerOffset: -2, DR: 4, ExpectedTXPowerOffset: -5, ExpectedDR: 5},
				{NStep: -1, TXPowerOffset: -2, DR: 4, ExpectedTXPowerOffset: 1, ExpectedDR: 4},
				{NStep: -1, TXPowerOffset: 0, DR: 4, ExpectedTXPowerOffset: 0, ExpectedDR: 4},
			}

			for i, tst := range testTable {
				Convey(fmt.Sprintf("Given NStep: %d, TXPowerOffset: %d, DR: %d [%d]", tst.NStep, tst.TXPowerOffset, tst.DR, i), func() {
					Convey(fmt.Sprintf("Then the ideal TXPowerOffset is %d and DR %d", tst.ExpectedTXPowerOffset, tst.ExpectedDR), func() {
						idealTXPowerOffset, idealDR := getIdealTXPowerOffsetAndDR(tst.NStep, tst.TXPowerOffset, tst.DR)
						So(idealTXPowerOffset, ShouldEqual, tst.ExpectedTXPowerOffset)
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

				macBlock := maccommand.Block{
					CID: lorawan.LinkADRReq,
					MACCommands: []lorawan.MACCommand{
						{
							CID: lorawan.LinkADRReq,
							Payload: &lorawan.LinkADRReqPayload{
								DataRate: 3,
								TXPower:  0,
								ChMask:   lorawan.ChMask{true, true, true}, // ADR applies to first three standard channels
								Redundancy: lorawan.Redundancy{
									ChMaskCntl: 0, // first block of 16 channels
									NbRep:      1,
								},
							},
						},
					},
				}

				macCFListBlock := maccommand.Block{
					CID: lorawan.LinkADRReq,
					MACCommands: []lorawan.MACCommand{
						{
							CID: lorawan.LinkADRReq,
							Payload: &lorawan.LinkADRReqPayload{
								DataRate: 3,
								TXPower:  0,
								ChMask:   lorawan.ChMask{true, true, true, true, true, false, true},
								Redundancy: lorawan.Redundancy{
									ChMaskCntl: 0, // first block of 16 channels
									NbRep:      1,
								},
							},
						},
					},
				}

				testTable := []struct {
					Name                    string
					NodeSession             *session.NodeSession
					RXPacket                models.RXPacket
					FullFCnt                uint32
					MACCommandQueue         []maccommand.Block
					ExpectedNodeSession     session.NodeSession
					ExpectedMACCommandQueue []maccommand.Block
					ExpectedError           error
				}{
					{
						Name: "ADR increasing data-rate by one step (no CFlist)",
						NodeSession: &session.NodeSession{
							DevAddr:            [4]byte{1, 2, 3, 4},
							DevEUI:             [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
							ADRInterval:        1,
							InstallationMargin: 5,
							EnabledChannels:    []int{0, 1, 2},
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
							UplinkHistory: []session.UplinkHistory{
								{FCnt: 1, MaxSNR: -7, GatewayCount: 1},
							},
							EnabledChannels: []int{0, 1, 2},
						},
						ExpectedMACCommandQueue: []maccommand.Block{
							macBlock,
						},
						ExpectedError: nil,
					},
					{
						Name: "ADR increasing data-rate by one step (no CFlist), but mac-command block pending",
						NodeSession: &session.NodeSession{
							DevAddr:            [4]byte{1, 2, 3, 4},
							DevEUI:             [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
							ADRInterval:        1,
							InstallationMargin: 5,
							EnabledChannels:    []int{0, 1, 2},
						},
						RXPacket: models.RXPacket{
							PHYPayload: phyPayloadADR,
							RXInfoSet: models.RXInfoSet{
								{DataRate: common.Band.DataRates[2], LoRaSNR: -7},
							},
						},
						FullFCnt: 1,
						MACCommandQueue: []maccommand.Block{
							{
								CID: lorawan.LinkADRReq,
								MACCommands: maccommand.MACCommands{
									lorawan.MACCommand{
										CID: lorawan.LinkADRReq,
										Payload: &lorawan.LinkADRReqPayload{
											ChMask: lorawan.ChMask{true, false, true, false, true, false},
										},
									},
								},
							},
						},
						ExpectedNodeSession: session.NodeSession{
							DevAddr:            [4]byte{1, 2, 3, 4},
							DevEUI:             [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
							ADRInterval:        1,
							InstallationMargin: 5,
							UplinkHistory: []session.UplinkHistory{
								{FCnt: 1, MaxSNR: -7, GatewayCount: 1},
							},
							EnabledChannels: []int{0, 1, 2},
						},
						ExpectedMACCommandQueue: []maccommand.Block{
							{
								CID: lorawan.LinkADRReq,
								MACCommands: maccommand.MACCommands{
									lorawan.MACCommand{
										CID: lorawan.LinkADRReq,
										Payload: &lorawan.LinkADRReqPayload{
											DataRate: 3,
											TXPower:  0,
											ChMask:   lorawan.ChMask{true, false, true, false, true, false},
											Redundancy: lorawan.Redundancy{
												ChMaskCntl: 0,
												NbRep:      1,
											},
										},
									},
								},
							},
						},
						ExpectedError: nil,
					},
					{
						Name: "ADR increasing data-rate by one step (extra channels added)",
						NodeSession: &session.NodeSession{
							DevAddr:            [4]byte{1, 2, 3, 4},
							DevEUI:             [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
							ADRInterval:        1,
							InstallationMargin: 5,
							EnabledChannels:    []int{0, 1, 2, 3, 4, 6},
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
							EnabledChannels:    []int{0, 1, 2, 3, 4, 6},
							UplinkHistory: []session.UplinkHistory{
								{FCnt: 1, MaxSNR: -7, GatewayCount: 1},
							},
						},
						ExpectedMACCommandQueue: []maccommand.Block{
							macCFListBlock,
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
						So(session.SaveNodeSession(p, *tst.NodeSession), ShouldBeNil)

						for _, block := range tst.MACCommandQueue {
							So(maccommand.AddQueueItem(p, tst.NodeSession.DevEUI, block), ShouldBeNil)
						}

						err := HandleADR(ctx, tst.NodeSession, tst.RXPacket, tst.FullFCnt)
						if tst.ExpectedError != nil {
							So(err, ShouldResemble, tst.ExpectedError)
							return
						}

						So(err, ShouldBeNil)
						So(tst.NodeSession, ShouldResemble, &tst.ExpectedNodeSession)

						macPayloadQueue, err := maccommand.ReadQueueItems(p, tst.NodeSession.DevEUI)
						So(err, ShouldBeNil)
						So(macPayloadQueue, ShouldResemble, tst.ExpectedMACCommandQueue)
					})
				}
			})
		})
	})
}
