package adr

import (
	"fmt"
	"testing"

	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/loraserver/internal/test"
	"github.com/brocaar/lorawan"
	. "github.com/smartystreets/goconvey/convey"
)

func TestADR(t *testing.T) {
	conf := test.GetConfig()
	conf.NetworkServer.NetworkSettings.InstallationMargin = 5
	if err := Setup(conf); err != nil {
		t.Fatal(err)
	}

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

		Convey("getMaxTXPowerOffsetIndex returns 7", func() {
			So(getMaxTXPowerOffsetIndex(), ShouldEqual, 7)
		})

		Convey("Testing getMaxSupportedTXPowerOffsetIndexForDevice", func() {
			Convey("When no MaxSupportedTXPowerIndex is set on the device session, it returns getMaxTXPowerOffsetIndex", func() {
				ds := storage.DeviceSession{}
				So(getMaxSupportedTXPowerOffsetIndexForDevice(ds), ShouldEqual, getMaxTXPowerOffsetIndex())
			})

			Convey("When MaxSupportedTXPowerIndex is set on the device session, this value is returned", func() {
				ds := storage.DeviceSession{
					MaxSupportedTXPowerIndex: 3,
				}
				So(getMaxSupportedTXPowerOffsetIndexForDevice(ds), ShouldEqual, ds.MaxSupportedTXPowerIndex)
				So(getMaxSupportedTXPowerOffsetIndexForDevice(ds), ShouldNotEqual, getMaxTXPowerOffsetIndex())
			})
		})

		Convey("Given a testtable for getIdealTXPowerAndDR", func() {
			testTable := []struct {
				Name                     string
				NStep                    int
				TXPowerIndex             int
				MaxSupportedDR           int
				MinSupportedTXPowerIndex int
				MaxSupportedTXPowerIndex int
				DR                       int
				ExpectedTXPowerIndex     int
				ExpectedDR               int
			}{
				{
					Name:                     "nothing to adjust",
					NStep:                    0,
					TXPowerIndex:             1,
					MaxSupportedDR:           5,
					MaxSupportedTXPowerIndex: getMaxTXPowerOffsetIndex(), // 5
					DR:                       3,
					ExpectedDR:               3,
					ExpectedTXPowerIndex:     1,
				},
				{
					Name:                     "one step: one step data-rate increase",
					NStep:                    1,
					TXPowerIndex:             1,
					MaxSupportedDR:           5,
					MaxSupportedTXPowerIndex: getMaxTXPowerOffsetIndex(), // 5
					DR:                       4,
					ExpectedDR:               5,
					ExpectedTXPowerIndex:     1,
				},
				{
					Name:                     "one step: one step tx-power decrease",
					NStep:                    1,
					TXPowerIndex:             1,
					MaxSupportedDR:           5,
					MaxSupportedTXPowerIndex: getMaxTXPowerOffsetIndex(), // 5
					DR:                       5,
					ExpectedDR:               5,
					ExpectedTXPowerIndex:     2,
				},
				{
					Name:                     "two steps: two steps data-rate increase",
					NStep:                    2,
					TXPowerIndex:             1,
					MaxSupportedDR:           5,
					MaxSupportedTXPowerIndex: getMaxTXPowerOffsetIndex(), // 5
					DR:                       3,
					ExpectedDR:               5,
					ExpectedTXPowerIndex:     1,
				},
				{
					Name:                     "two steps: one step data-rate increase (due to max supported dr), one step tx-power decrease",
					NStep:                    2,
					TXPowerIndex:             1,
					MaxSupportedDR:           4,
					MaxSupportedTXPowerIndex: getMaxTXPowerOffsetIndex(), // 5
					DR:                       3,
					ExpectedDR:               4,
					ExpectedTXPowerIndex:     2,
				},
				{
					Name:                     "two steps: one step data-rate increase, one step tx-power decrease",
					NStep:                    2,
					TXPowerIndex:             1,
					MaxSupportedDR:           5,
					MaxSupportedTXPowerIndex: getMaxTXPowerOffsetIndex(), // 5
					DR:                       4,
					ExpectedDR:               5,
					ExpectedTXPowerIndex:     2,
				},
				{
					Name:                     "two steps: two steps tx-power decrease",
					NStep:                    2,
					TXPowerIndex:             1,
					MaxSupportedDR:           5,
					MaxSupportedTXPowerIndex: getMaxTXPowerOffsetIndex(), // 5
					DR:                       5,
					ExpectedDR:               5,
					ExpectedTXPowerIndex:     3,
				},
				{
					Name:                     "two steps: one step tx-power decrease due to max supported tx power index",
					NStep:                    2,
					TXPowerIndex:             1,
					MaxSupportedDR:           5,
					MaxSupportedTXPowerIndex: 2,
					DR:                       5,
					ExpectedDR:               5,
					ExpectedTXPowerIndex:     2,
				},
				{
					Name:                     "one negative step: one step power increase",
					NStep:                    -1,
					TXPowerIndex:             1,
					MaxSupportedDR:           5,
					MaxSupportedTXPowerIndex: getMaxTXPowerOffsetIndex(), // 5
					DR:                       4,
					ExpectedDR:               4,
					ExpectedTXPowerIndex:     0,
				},
				{
					Name:                     "one negative step, nothing to do (adr engine will never decrease data-rate)",
					NStep:                    -1,
					TXPowerIndex:             0,
					MaxSupportedDR:           5,
					MaxSupportedTXPowerIndex: getMaxTXPowerOffsetIndex(), // 5
					DR:                       4,
					ExpectedDR:               4,
					ExpectedTXPowerIndex:     0,
				},
				{
					Name:                     "10 negative steps, should not adjust anything (as we already reached the min tx-power index)",
					NStep:                    -10,
					TXPowerIndex:             1,
					MinSupportedTXPowerIndex: 1,
					DR:                       4,
					ExpectedDR:               4,
					ExpectedTXPowerIndex:     1,
				},
			}

			for i, tst := range testTable {
				Convey(fmt.Sprintf("Testing '%s' with NStep: %d, TXPowerOffsetIndex: %d, DR: %d [%d]", tst.Name, tst.NStep, tst.TXPowerIndex, tst.DR, i), func() {
					Convey(fmt.Sprintf("Then the ideal TXPowerOffsetIndex is %d and DR %d", tst.ExpectedTXPowerIndex, tst.ExpectedDR), func() {
						idealTXPowerIndex, idealDR := getIdealTXPowerOffsetAndDR(tst.NStep, tst.TXPowerIndex, tst.DR, tst.MinSupportedTXPowerIndex, tst.MaxSupportedTXPowerIndex, tst.MaxSupportedDR)
						So(idealTXPowerIndex, ShouldEqual, tst.ExpectedTXPowerIndex)
						So(idealDR, ShouldEqual, tst.ExpectedDR)
					})
				})
			}
		})

		Convey("Given a clean Redis database", func() {
			if err := storage.Setup(conf); err != nil {
				panic(err)
			}
			test.MustFlushRedis(storage.RedisPool())

			Convey("Given a testtable for HandleADR", func() {
				macBlock := storage.MACCommandBlock{
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

				macCFListBlock := storage.MACCommandBlock{
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
					Name            string
					ServiceProfile  storage.ServiceProfile
					DeviceSession   storage.DeviceSession
					LinkADRReqBlock *storage.MACCommandBlock
					Expected        []storage.MACCommandBlock
					ExpectedError   error
				}{
					{
						Name: "ADR increasing data-rate by one step (no CFlist)",
						ServiceProfile: storage.ServiceProfile{
							DRMin: 0,
							DRMax: 5,
						},
						DeviceSession: storage.DeviceSession{
							DevAddr:               [4]byte{1, 2, 3, 4},
							DevEUI:                [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
							EnabledUplinkChannels: []int{0, 1, 2},
							DR:                    2,
							ADR:                   true,
							UplinkHistory: []storage.UplinkHistory{
								{MaxSNR: -7},
							},
						},
						Expected:      []storage.MACCommandBlock{macBlock},
						ExpectedError: nil,
					},
					{
						Name: "ADR decreasing data-rate by one step as a lower value has been specified in the service-profile",
						ServiceProfile: storage.ServiceProfile{
							DRMin: 0,
							DRMax: 4,
						},
						DeviceSession: storage.DeviceSession{
							DevAddr:               [4]byte{1, 2, 3, 4},
							DevEUI:                [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
							EnabledUplinkChannels: []int{0, 1, 2},
							DR:                    5,
							ADR:                   true,
							UplinkHistory: []storage.UplinkHistory{
								{MaxSNR: 20},
							},
						},
						Expected: []storage.MACCommandBlock{
							{
								CID: lorawan.LinkADRReq,
								MACCommands: []lorawan.MACCommand{
									{
										CID: lorawan.LinkADRReq,
										Payload: &lorawan.LinkADRReqPayload{
											DataRate: 4,
											TXPower:  0,
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
						Name: "ADR increasing tx-power by one step (no CFlist)",
						ServiceProfile: storage.ServiceProfile{
							DRMin: 0,
							DRMax: 5,
						},
						DeviceSession: storage.DeviceSession{
							DevAddr:               [4]byte{1, 2, 3, 4},
							DevEUI:                [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
							EnabledUplinkChannels: []int{0, 1, 2},
							DR:                    5,
							TXPowerIndex:          3,
							ADR:                   true,
							UplinkHistory: []storage.UplinkHistory{
								{MaxSNR: 1, TXPowerIndex: 3},
							},
						},
						Expected: []storage.MACCommandBlock{
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
					},
					{
						// this is because we don't have enough uplink history
						// and the packetloss function returns therefore 0%.
						Name: "ADR decreasing NbTrans by one",
						ServiceProfile: storage.ServiceProfile{
							DRMin: 0,
							DRMax: 5,
						},
						DeviceSession: storage.DeviceSession{
							DevAddr:               [4]byte{1, 2, 3, 4},
							DevEUI:                [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
							EnabledUplinkChannels: []int{0, 1, 2},
							DR:                    5,
							TXPowerIndex:          4,
							NbTrans:               3,
							ADR:                   true,
							UplinkHistory: []storage.UplinkHistory{
								{MaxSNR: -5, TXPowerIndex: 4},
							},
						},
						Expected: []storage.MACCommandBlock{
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
					},
					{
						Name: "ADR increasing data-rate by one step (no CFlist), updating given LinkADRReq block",
						ServiceProfile: storage.ServiceProfile{
							DRMin: 0,
							DRMax: 5,
						},
						DeviceSession: storage.DeviceSession{
							DevAddr:               [4]byte{1, 2, 3, 4},
							DevEUI:                [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
							EnabledUplinkChannels: []int{0, 1, 2},
							DR:                    2,
							ADR:                   true,
							UplinkHistory: []storage.UplinkHistory{
								{MaxSNR: -7, TXPowerIndex: 0},
							},
						},
						LinkADRReqBlock: &storage.MACCommandBlock{
							CID: lorawan.LinkADRReq,
							MACCommands: storage.MACCommands{
								lorawan.MACCommand{
									CID: lorawan.LinkADRReq,
									Payload: &lorawan.LinkADRReqPayload{
										ChMask: lorawan.ChMask{true, false, true, false, true, false},
									},
								},
							},
						},
						Expected: []storage.MACCommandBlock{
							{
								CID: lorawan.LinkADRReq,
								MACCommands: storage.MACCommands{
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
					},
					{
						Name: "ADR increasing data-rate by one step (extra channels added)",
						ServiceProfile: storage.ServiceProfile{
							DRMin: 0,
							DRMax: 5,
						},
						DeviceSession: storage.DeviceSession{
							DevAddr:               [4]byte{1, 2, 3, 4},
							DevEUI:                [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
							EnabledUplinkChannels: []int{0, 1, 2, 3, 4, 6},
							DR:                    2,
							ADR:                   true,
							UplinkHistory: []storage.UplinkHistory{
								{MaxSNR: -7, TXPowerIndex: 0},
							},
						},
						Expected: []storage.MACCommandBlock{
							macCFListBlock,
						},
						ExpectedError: nil,
					},
					{
						Name: "data-rate can be increased, but no ADR flag set",
						ServiceProfile: storage.ServiceProfile{
							DRMin: 0,
							DRMax: 5,
						},
						DeviceSession: storage.DeviceSession{
							DevAddr: [4]byte{1, 2, 3, 4},
							DevEUI:  [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
							DR:      2,
							ADR:     false,
							UplinkHistory: []storage.UplinkHistory{
								{MaxSNR: -7, TXPowerIndex: 0},
							},
						},
						ExpectedError: nil,
					},
					{
						Name: "ADR increasing data-rate by one step (through history table)",
						ServiceProfile: storage.ServiceProfile{
							DRMin: 0,
							DRMax: 5,
						},
						DeviceSession: storage.DeviceSession{
							DevAddr:               [4]byte{1, 2, 3, 4},
							DevEUI:                [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
							EnabledUplinkChannels: []int{0, 1, 2},
							UplinkHistory: []storage.UplinkHistory{
								{MaxSNR: -7, TXPowerIndex: 0},
								{MaxSNR: -15, TXPowerIndex: 0},
							},
							DR:  2,
							ADR: true,
						},
						Expected:      []storage.MACCommandBlock{macBlock},
						ExpectedError: nil,
					},
					{
						Name: "ADR not increasing tx power (as the TX power in the history table does not match)",
						ServiceProfile: storage.ServiceProfile{
							DRMin: 0,
							DRMax: 5,
						},
						DeviceSession: storage.DeviceSession{
							DevAddr:               [4]byte{1, 2, 3, 4},
							DevEUI:                [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
							EnabledUplinkChannels: []int{0, 1, 2},
							DR:                    5,
							TXPowerIndex:          3,
							NbTrans:               1,
							UplinkHistory: []storage.UplinkHistory{
								{MaxSNR: 7, TXPowerIndex: 0},
								{MaxSNR: -5, TXPowerIndex: 3},
							},
							ADR: true,
						},
						ExpectedError: nil,
					},
					{
						Name: "ADR not decreasing tx power (as we don't have a full history table for the currently used TXPower)",
						ServiceProfile: storage.ServiceProfile{
							DRMin: 0,
							DRMax: 5,
						},
						DeviceSession: storage.DeviceSession{
							DevAddr:               [4]byte{1, 2, 3, 4},
							DevEUI:                [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
							EnabledUplinkChannels: []int{0, 1, 2},
							DR:                    5,
							TXPowerIndex:          3,
							NbTrans:               1,
							UplinkHistory: []storage.UplinkHistory{
								{MaxSNR: -20, TXPowerIndex: 0},
								{MaxSNR: -20, TXPowerIndex: 3},
								{MaxSNR: -20, TXPowerIndex: 3},
								{MaxSNR: -20, TXPowerIndex: 3},
								{MaxSNR: -20, TXPowerIndex: 3},
								{MaxSNR: -20, TXPowerIndex: 3},
								{MaxSNR: -20, TXPowerIndex: 3},
								{MaxSNR: -20, TXPowerIndex: 3},
								{MaxSNR: -20, TXPowerIndex: 3},
								{MaxSNR: -20, TXPowerIndex: 3},
								{MaxSNR: -20, TXPowerIndex: 3},
								{MaxSNR: -20, TXPowerIndex: 3},
								{MaxSNR: -20, TXPowerIndex: 3},
								{MaxSNR: -20, TXPowerIndex: 3},
								{MaxSNR: -20, TXPowerIndex: 3},
								{MaxSNR: -20, TXPowerIndex: 3},
								{MaxSNR: -20, TXPowerIndex: 3},
								{MaxSNR: -20, TXPowerIndex: 3},
								{MaxSNR: -20, TXPowerIndex: 3},
								{MaxSNR: -20, TXPowerIndex: 3},
							},
							ADR: true,
						},
						ExpectedError: nil,
					},
					{
						Name: "ADR decreasing tx power",
						ServiceProfile: storage.ServiceProfile{
							DRMin: 0,
							DRMax: 5,
						},
						DeviceSession: storage.DeviceSession{
							DevAddr:               [4]byte{1, 2, 3, 4},
							DevEUI:                [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
							EnabledUplinkChannels: []int{0, 1, 2},
							DR:                    5,
							TXPowerIndex:          3,
							NbTrans:               1,
							UplinkHistory: []storage.UplinkHistory{
								{FCnt: 0, MaxSNR: -20, TXPowerIndex: 3},
								{FCnt: 1, MaxSNR: -20, TXPowerIndex: 3},
								{FCnt: 2, MaxSNR: -20, TXPowerIndex: 3},
								{FCnt: 3, MaxSNR: -20, TXPowerIndex: 3},
								{FCnt: 4, MaxSNR: -20, TXPowerIndex: 3},
								{FCnt: 5, MaxSNR: -20, TXPowerIndex: 3},
								{FCnt: 6, MaxSNR: -20, TXPowerIndex: 3},
								{FCnt: 7, MaxSNR: -20, TXPowerIndex: 3},
								{FCnt: 8, MaxSNR: -20, TXPowerIndex: 3},
								{FCnt: 9, MaxSNR: -20, TXPowerIndex: 3},
								{FCnt: 10, MaxSNR: -20, TXPowerIndex: 3},
								{FCnt: 11, MaxSNR: -20, TXPowerIndex: 3},
								{FCnt: 12, MaxSNR: -20, TXPowerIndex: 3},
								{FCnt: 13, MaxSNR: -20, TXPowerIndex: 3},
								{FCnt: 14, MaxSNR: -20, TXPowerIndex: 3},
								{FCnt: 15, MaxSNR: -20, TXPowerIndex: 3},
								{FCnt: 16, MaxSNR: -20, TXPowerIndex: 3},
								{FCnt: 17, MaxSNR: -20, TXPowerIndex: 3},
								{FCnt: 18, MaxSNR: -20, TXPowerIndex: 3},
								{FCnt: 19, MaxSNR: -20, TXPowerIndex: 3},
							},
							ADR: true,
						},
						Expected: []storage.MACCommandBlock{
							{
								CID: lorawan.LinkADRReq,
								MACCommands: []lorawan.MACCommand{
									{
										CID: lorawan.LinkADRReq,
										Payload: &lorawan.LinkADRReqPayload{
											DataRate: 5,
											TXPower:  0,
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
				}

				for i, tst := range testTable {
					Convey(fmt.Sprintf("Test: %s [%d]", tst.Name, i), func() {
						blocks, err := HandleADR(tst.ServiceProfile, tst.DeviceSession, tst.LinkADRReqBlock)
						if tst.ExpectedError != nil {
							So(err, ShouldNotBeNil)
							So(err, ShouldResemble, tst.ExpectedError)
						}
						So(err, ShouldBeNil)

						So(blocks, ShouldResemble, tst.Expected)
					})
				}
			})

			Convey("Given an ADR request when ADR is disabled", func() {
				conf.NetworkServer.NetworkSettings.DisableADR = true
				if err := Setup(conf); err != nil {
					t.Fatal(err)
				}

				sp := storage.ServiceProfile{
					DRMin: 0,
					DRMax: 5,
				}

				ds := storage.DeviceSession{
					DevAddr:               [4]byte{1, 2, 3, 4},
					DevEUI:                [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
					EnabledUplinkChannels: []int{0, 1, 2},
					DR:                    5,
					TXPowerIndex:          3,
					ADR:                   true,
					UplinkHistory: []storage.UplinkHistory{
						{MaxSNR: 1, TXPowerIndex: 3},
					},
				}

				larb := &storage.MACCommandBlock{
					CID: lorawan.LinkADRReq,
					MACCommands: storage.MACCommands{
						lorawan.MACCommand{
							CID: lorawan.LinkADRReq,
							Payload: &lorawan.LinkADRReqPayload{
								ChMask: lorawan.ChMask{true, true, true},
							},
						},
					},
				}

				blocks, err := HandleADR(sp, ds, larb)

				So(err, ShouldBeNil)
				So(blocks, ShouldBeNil)
			})
		})
	})
}
