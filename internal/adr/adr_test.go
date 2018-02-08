package adr

import (
	"fmt"
	"testing"

	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/maccommand"
	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/loraserver/internal/test"
	"github.com/brocaar/lorawan"
	. "github.com/smartystreets/goconvey/convey"
)

func TestADR(t *testing.T) {
	conf := test.GetConfig()
	common.InstallationMargin = 5

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

		Convey("Testing getMaxSupportedDRForNode", func() {
			Convey("When no MaxSupportedDR is set on the device session, it returns getMaxAllowedDR", func() {
				ds := storage.DeviceSession{}
				So(getMaxSupportedDRForNode(&ds), ShouldEqual, getMaxAllowedDR())
			})

			Convey("When MaxSupportedDR is set on the device session, this value is returned", func() {
				ds := storage.DeviceSession{
					MaxSupportedDR: 3,
				}
				So(getMaxSupportedDRForNode(&ds), ShouldEqual, ds.MaxSupportedDR)
				So(getMaxSupportedDRForNode(&ds), ShouldNotEqual, getMaxAllowedDR())
			})
		})

		Convey("getMaxTXPowerOffsetIndex returns 7", func() {
			So(getMaxTXPowerOffsetIndex(), ShouldEqual, 7)
		})

		Convey("Testing getMaxSupportedTXPowerOffsetIndexForNode", func() {
			Convey("When no MaxSupportedTXPowerIndex is set on the device session, it returns getMaxTXPowerOffsetIndex", func() {
				ds := storage.DeviceSession{}
				So(getMaxSupportedTXPowerOffsetIndexForNode(&ds), ShouldEqual, getMaxTXPowerOffsetIndex())
			})

			Convey("When MaxSupportedTXPowerIndex is set on the device session, this value is returned", func() {
				ds := storage.DeviceSession{
					MaxSupportedTXPowerIndex: 3,
				}
				So(getMaxSupportedTXPowerOffsetIndexForNode(&ds), ShouldEqual, ds.MaxSupportedTXPowerIndex)
				So(getMaxSupportedTXPowerOffsetIndexForNode(&ds), ShouldNotEqual, getMaxTXPowerOffsetIndex())
			})
		})

		Convey("Given a testtable for getIdealTXPowerAndDR", func() {
			testTable := []struct {
				Name                     string
				NStep                    int
				TXPowerIndex             int
				MaxSupportedDR           int
				MaxSupportedTXPowerIndex int
				DR                       int
				ExpectedTXPowerIndex     int
				ExpectedDR               int
			}{
				{
					Name:                     "nothing to adjust",
					NStep:                    0,
					TXPowerIndex:             1,
					MaxSupportedDR:           getMaxAllowedDR(),          // 5
					MaxSupportedTXPowerIndex: getMaxTXPowerOffsetIndex(), // 5
					DR:                   3,
					ExpectedDR:           3,
					ExpectedTXPowerIndex: 1,
				},
				{
					Name:                     "one step: one step data-rate increase",
					NStep:                    1,
					TXPowerIndex:             1,
					MaxSupportedDR:           getMaxAllowedDR(),
					MaxSupportedTXPowerIndex: getMaxTXPowerOffsetIndex(), // 5
					DR:                   4,
					ExpectedDR:           5,
					ExpectedTXPowerIndex: 1,
				},
				{
					Name:                     "one step: one step tx-power decrease",
					NStep:                    1,
					TXPowerIndex:             1,
					MaxSupportedDR:           getMaxAllowedDR(),
					MaxSupportedTXPowerIndex: getMaxTXPowerOffsetIndex(), // 5
					DR:                   5,
					ExpectedDR:           5,
					ExpectedTXPowerIndex: 2,
				},
				{
					Name:                     "two steps: two steps data-rate increase",
					NStep:                    2,
					TXPowerIndex:             1,
					MaxSupportedDR:           getMaxAllowedDR(),
					MaxSupportedTXPowerIndex: getMaxTXPowerOffsetIndex(), // 5
					DR:                   3,
					ExpectedDR:           5,
					ExpectedTXPowerIndex: 1,
				},
				{
					Name:                     "two steps: one step data-rate increase (due to max supported dr), one step tx-power decrease",
					NStep:                    2,
					TXPowerIndex:             1,
					MaxSupportedDR:           4,
					MaxSupportedTXPowerIndex: getMaxTXPowerOffsetIndex(), // 5
					DR:                   3,
					ExpectedDR:           4,
					ExpectedTXPowerIndex: 2,
				},
				{
					Name:                     "two steps: one step data-rate increase, one step tx-power decrease",
					NStep:                    2,
					TXPowerIndex:             1,
					MaxSupportedDR:           getMaxAllowedDR(),
					MaxSupportedTXPowerIndex: getMaxTXPowerOffsetIndex(), // 5
					DR:                   4,
					ExpectedDR:           5,
					ExpectedTXPowerIndex: 2,
				},
				{
					Name:                     "two steps: two steps tx-power decrease",
					NStep:                    2,
					TXPowerIndex:             1,
					MaxSupportedDR:           getMaxAllowedDR(),
					MaxSupportedTXPowerIndex: getMaxTXPowerOffsetIndex(), // 5
					DR:                   5,
					ExpectedDR:           5,
					ExpectedTXPowerIndex: 3,
				},
				{
					Name:                     "two steps: one step tx-power decrease due to max supported tx power index",
					NStep:                    2,
					TXPowerIndex:             1,
					MaxSupportedDR:           getMaxAllowedDR(),
					MaxSupportedTXPowerIndex: 2,
					DR:                   5,
					ExpectedDR:           5,
					ExpectedTXPowerIndex: 2,
				},
				{
					Name:                     "one negative step: one step power increase",
					NStep:                    -1,
					TXPowerIndex:             1,
					MaxSupportedDR:           getMaxAllowedDR(),
					MaxSupportedTXPowerIndex: getMaxTXPowerOffsetIndex(), // 5
					DR:                   4,
					ExpectedDR:           4,
					ExpectedTXPowerIndex: 0,
				},
				{
					Name:                     "one negative step, nothing to do (adr engine will never decrease data-rate)",
					NStep:                    -1,
					TXPowerIndex:             0,
					MaxSupportedDR:           getMaxAllowedDR(),
					MaxSupportedTXPowerIndex: getMaxTXPowerOffsetIndex(), // 5
					DR:                   4,
					ExpectedDR:           4,
					ExpectedTXPowerIndex: 0,
				},
			}

			for i, tst := range testTable {
				Convey(fmt.Sprintf("Testing '%s' with NStep: %d, TXPowerOffsetIndex: %d, DR: %d [%d]", tst.Name, tst.NStep, tst.TXPowerIndex, tst.DR, i), func() {
					Convey(fmt.Sprintf("Then the ideal TXPowerOffsetIndex is %d and DR %d", tst.ExpectedTXPowerIndex, tst.ExpectedDR), func() {
						idealTXPowerIndex, idealDR := getIdealTXPowerOffsetAndDR(tst.NStep, tst.TXPowerIndex, tst.DR, tst.MaxSupportedTXPowerIndex, tst.MaxSupportedDR)
						So(idealTXPowerIndex, ShouldEqual, tst.ExpectedTXPowerIndex)
						So(idealDR, ShouldEqual, tst.ExpectedDR)
					})
				})
			}
		})

		Convey("Given a clean Redis database", func() {
			common.RedisPool = common.NewRedisPool(conf.RedisURL)
			test.MustFlushRedis(common.RedisPool)

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
					DeviceSession           *storage.DeviceSession
					RXPacket                models.RXPacket
					FullFCnt                uint32
					MACCommandQueue         []maccommand.Block
					ExpectedDeviceSession   storage.DeviceSession
					ExpectedMACCommandQueue []maccommand.Block
					ExpectedError           error
				}{
					{
						Name: "ADR increasing data-rate by one step (no CFlist)",
						DeviceSession: &storage.DeviceSession{
							DevAddr:         [4]byte{1, 2, 3, 4},
							DevEUI:          [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
							EnabledChannels: []int{0, 1, 2},
						},
						RXPacket: models.RXPacket{
							PHYPayload: phyPayloadADR,
							TXInfo: models.TXInfo{
								DataRate: common.Band.DataRates[2],
							},
							RXInfoSet: models.RXInfoSet{
								{LoRaSNR: -7},
							},
						},
						FullFCnt: 1,
						ExpectedDeviceSession: storage.DeviceSession{
							DevAddr: [4]byte{1, 2, 3, 4},
							DevEUI:  [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
							UplinkHistory: []storage.UplinkHistory{
								{FCnt: 1, MaxSNR: -7, GatewayCount: 1},
							},
							EnabledChannels: []int{0, 1, 2},
							DR:              2,
						},
						ExpectedMACCommandQueue: []maccommand.Block{
							macBlock,
						},
						ExpectedError: nil,
					},
					{
						Name: "ADR increasing tx-power by one step (no CFlist)",
						DeviceSession: &storage.DeviceSession{
							DevAddr:         [4]byte{1, 2, 3, 4},
							DevEUI:          [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
							EnabledChannels: []int{0, 1, 2},
							DR:              5,
							TXPowerIndex:    3,
						},
						RXPacket: models.RXPacket{
							PHYPayload: phyPayloadADR,
							TXInfo: models.TXInfo{
								DataRate: common.Band.DataRates[5],
							},
							RXInfoSet: models.RXInfoSet{
								{LoRaSNR: 1},
							},
						},
						FullFCnt: 1,
						ExpectedDeviceSession: storage.DeviceSession{
							DevAddr: [4]byte{1, 2, 3, 4},
							DevEUI:  [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
							UplinkHistory: []storage.UplinkHistory{
								{FCnt: 1, MaxSNR: 1, GatewayCount: 1},
							},
							EnabledChannels: []int{0, 1, 2},
							DR:              5,
							TXPowerIndex:    3,
						},
						ExpectedMACCommandQueue: []maccommand.Block{
							{
								CID: lorawan.LinkADRReq,
								MACCommands: []lorawan.MACCommand{
									{
										CID: lorawan.LinkADRReq,
										Payload: &lorawan.LinkADRReqPayload{
											DataRate: 5,
											TXPower:  4,
											ChMask:   lorawan.ChMask{true, true, true},
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
						// this is because we don't have enough uplink history
						// and the packetloss function returns therefore 0%.
						Name: "ADR decreasing NbTrans by one",
						DeviceSession: &storage.DeviceSession{
							DevAddr:         [4]byte{1, 2, 3, 4},
							DevEUI:          [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
							EnabledChannels: []int{0, 1, 2},
							DR:              5,
							TXPowerIndex:    4,
							NbTrans:         3,
						},
						RXPacket: models.RXPacket{
							PHYPayload: phyPayloadADR,
							TXInfo: models.TXInfo{
								DataRate: common.Band.DataRates[5],
							},
							RXInfoSet: models.RXInfoSet{
								{LoRaSNR: -5},
							},
						},
						FullFCnt: 1,
						ExpectedDeviceSession: storage.DeviceSession{
							DevAddr: [4]byte{1, 2, 3, 4},
							DevEUI:  [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
							UplinkHistory: []storage.UplinkHistory{
								{FCnt: 1, MaxSNR: -5, GatewayCount: 1},
							},
							EnabledChannels: []int{0, 1, 2},
							DR:              5,
							TXPowerIndex:    4,
							NbTrans:         3,
						},
						ExpectedMACCommandQueue: []maccommand.Block{
							{
								CID: lorawan.LinkADRReq,
								MACCommands: []lorawan.MACCommand{
									{
										CID: lorawan.LinkADRReq,
										Payload: &lorawan.LinkADRReqPayload{
											DataRate: 5,
											TXPower:  4,
											ChMask:   lorawan.ChMask{true, true, true},
											Redundancy: lorawan.Redundancy{
												ChMaskCntl: 0,
												NbRep:      2,
											},
										},
									},
								},
							},
						},
						ExpectedError: nil,
					},
					{
						Name: "ADR increasing data-rate by one step and tx-power reset due to node changing its data-rate (no CFlist)",
						DeviceSession: &storage.DeviceSession{
							DevAddr:         [4]byte{1, 2, 3, 4},
							DevEUI:          [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
							EnabledChannels: []int{0, 1, 2},
							DR:              5,
							TXPowerIndex:    3,
						},
						RXPacket: models.RXPacket{
							PHYPayload: phyPayloadADR,
							TXInfo: models.TXInfo{
								DataRate: common.Band.DataRates[2],
							},
							RXInfoSet: models.RXInfoSet{
								{LoRaSNR: -7},
							},
						},
						FullFCnt: 1,
						ExpectedDeviceSession: storage.DeviceSession{
							DevAddr: [4]byte{1, 2, 3, 4},
							DevEUI:  [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
							UplinkHistory: []storage.UplinkHistory{
								{FCnt: 1, MaxSNR: -7, GatewayCount: 1},
							},
							EnabledChannels: []int{0, 1, 2},
							DR:              2,
							TXPowerIndex:    0,
						},
						ExpectedMACCommandQueue: []maccommand.Block{
							macBlock,
						},
						ExpectedError: nil,
					},
					{
						Name: "ADR increasing data-rate by one step (no CFlist), but mac-command block pending",
						DeviceSession: &storage.DeviceSession{
							DevAddr:         [4]byte{1, 2, 3, 4},
							DevEUI:          [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
							EnabledChannels: []int{0, 1, 2},
						},
						RXPacket: models.RXPacket{
							PHYPayload: phyPayloadADR,
							TXInfo: models.TXInfo{
								DataRate: common.Band.DataRates[2],
							},
							RXInfoSet: models.RXInfoSet{
								{LoRaSNR: -7},
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
						ExpectedDeviceSession: storage.DeviceSession{
							DevAddr: [4]byte{1, 2, 3, 4},
							DevEUI:  [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
							UplinkHistory: []storage.UplinkHistory{
								{FCnt: 1, MaxSNR: -7, GatewayCount: 1},
							},
							EnabledChannels: []int{0, 1, 2},
							DR:              2,
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
						DeviceSession: &storage.DeviceSession{
							DevAddr:         [4]byte{1, 2, 3, 4},
							DevEUI:          [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
							EnabledChannels: []int{0, 1, 2, 3, 4, 6},
						},
						RXPacket: models.RXPacket{
							PHYPayload: phyPayloadADR,
							TXInfo: models.TXInfo{
								DataRate: common.Band.DataRates[2],
							},
							RXInfoSet: models.RXInfoSet{
								{LoRaSNR: -7},
							},
						},
						FullFCnt: 1,
						ExpectedDeviceSession: storage.DeviceSession{
							DevAddr:         [4]byte{1, 2, 3, 4},
							DevEUI:          [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
							EnabledChannels: []int{0, 1, 2, 3, 4, 6},
							UplinkHistory: []storage.UplinkHistory{
								{FCnt: 1, MaxSNR: -7, GatewayCount: 1},
							},
							DR: 2,
						},
						ExpectedMACCommandQueue: []maccommand.Block{
							macCFListBlock,
						},
						ExpectedError: nil,
					},
					{
						Name: "data-rate can be increased, but no ADR flag set",
						DeviceSession: &storage.DeviceSession{
							DevAddr: [4]byte{1, 2, 3, 4},
							DevEUI:  [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
						},
						RXPacket: models.RXPacket{
							PHYPayload: phyPayloadNoADR,
							TXInfo: models.TXInfo{
								DataRate: common.Band.DataRates[2],
							},
							RXInfoSet: models.RXInfoSet{
								{LoRaSNR: -7},
							},
						},
						FullFCnt: 1,
						ExpectedDeviceSession: storage.DeviceSession{
							DevAddr: [4]byte{1, 2, 3, 4},
							DevEUI:  [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
							UplinkHistory: []storage.UplinkHistory{
								{FCnt: 1, MaxSNR: -7, GatewayCount: 1},
							},
							DR: 2,
						},
						ExpectedError: nil,
					},
				}

				for i, tst := range testTable {
					Convey(fmt.Sprintf("Test: %s [%d]", tst.Name, i), func() {
						So(storage.SaveDeviceSession(common.RedisPool, *tst.DeviceSession), ShouldBeNil)

						for _, block := range tst.MACCommandQueue {
							So(maccommand.AddQueueItem(common.RedisPool, tst.DeviceSession.DevEUI, block), ShouldBeNil)
						}

						err := HandleADR(tst.DeviceSession, tst.RXPacket, tst.FullFCnt)
						if tst.ExpectedError != nil {
							So(err, ShouldResemble, tst.ExpectedError)
							return
						}

						So(err, ShouldBeNil)
						So(tst.DeviceSession, ShouldResemble, &tst.ExpectedDeviceSession)

						macPayloadQueue, err := maccommand.ReadQueueItems(common.RedisPool, tst.DeviceSession.DevEUI)
						So(err, ShouldBeNil)
						So(macPayloadQueue, ShouldResemble, tst.ExpectedMACCommandQueue)
					})
				}
			})
		})
	})
}
