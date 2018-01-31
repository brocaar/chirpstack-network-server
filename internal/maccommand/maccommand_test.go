package maccommand

import (
	"fmt"
	"testing"

	"github.com/Frankz/loraserver/internal/common"
	"github.com/Frankz/loraserver/internal/models"
	"github.com/Frankz/loraserver/internal/storage"
	"github.com/Frankz/loraserver/internal/test"
	"github.com/Frankz/lorawan"
	"github.com/Frankz/lorawan/band"
	. "github.com/smartystreets/goconvey/convey"
)

func TestLinkCheckReq(t *testing.T) {
	conf := test.GetConfig()

	Convey("Given a clean Redis database", t, func() {
		common.RedisPool = common.NewRedisPool(conf.RedisURL)
		test.MustFlushRedis(common.RedisPool)

		Convey("Given a device-session", func() {
			ds := storage.DeviceSession{
				DevEUI:          [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
				EnabledChannels: []int{0, 1},
			}
			So(storage.SaveDeviceSession(common.RedisPool, ds), ShouldBeNil)

			Convey("Test DevStatusAns", func() {
				block := Block{
					CID: lorawan.DevStatusAns,
					MACCommands: MACCommands{
						lorawan.MACCommand{
							CID: lorawan.DevStatusAns,
							Payload: &lorawan.DevStatusAnsPayload{
								Battery: 200,
								Margin:  21,
							},
						},
					},
				}

				So(Handle(&ds, block, nil, models.RXInfoSet{}), ShouldBeNil)

				Convey("Then the dev-status fields on the device-session are updated", func() {
					So(ds.LastDevStatusBattery, ShouldEqual, 200)
					So(ds.LastDevStatusMargin, ShouldEqual, 21)
				})
			})

			Convey("Test LinkCheckReq", func() {
				block := Block{
					CID: lorawan.LinkCheckReq,
					MACCommands: MACCommands{
						lorawan.MACCommand{
							CID: lorawan.LinkCheckReq,
						},
					},
				}

				rxInfoSet := models.RXInfoSet{
					{
						LoRaSNR: 5,
						DataRate: band.DataRate{
							SpreadFactor: 10,
						},
					},
				}

				So(Handle(&ds, block, nil, rxInfoSet), ShouldBeNil)

				Convey("Then the expected response was added to the mac-command queue", func() {
					items, err := ReadQueueItems(common.RedisPool, ds.DevEUI)
					So(err, ShouldBeNil)
					So(items, ShouldHaveLength, 1)
					So(items[0], ShouldResemble, Block{
						CID: lorawan.LinkCheckAns,
						MACCommands: MACCommands{
							{
								CID: lorawan.LinkCheckAns,
								Payload: &lorawan.LinkCheckAnsPayload{
									GwCnt:  1,
									Margin: 20, // 5 - -15 (see SpreadFactorToRequiredSNRTable)
								},
							},
						},
					})
				})
			})
		})
	})
}

func TestLinkADRAns(t *testing.T) {
	conf := test.GetConfig()

	Convey("Given a clean Redis database", t, func() {
		common.RedisPool = common.NewRedisPool(conf.RedisURL)
		test.MustFlushRedis(common.RedisPool)

		Convey("Given a device-session", func() {
			ds := storage.DeviceSession{
				DevEUI:          [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
				EnabledChannels: []int{0, 1},
			}
			So(storage.SaveDeviceSession(common.RedisPool, ds), ShouldBeNil)

			Convey("Testing LinkADRAns", func() {
				testTable := []struct {
					Name                  string
					DeviceSession         storage.DeviceSession
					LinkADRReqPayload     *lorawan.LinkADRReqPayload
					LinkADRAnsPayload     lorawan.LinkADRAnsPayload
					ExpectedDeviceSession storage.DeviceSession
					ExpectedError         error
				}{
					{
						Name: "pending request and positive ACK updates tx-power, nbtrans and channels",
						DeviceSession: storage.DeviceSession{
							EnabledChannels: []int{0, 1},
						},
						LinkADRReqPayload: &lorawan.LinkADRReqPayload{
							ChMask:   lorawan.ChMask{true, true, true},
							DataRate: 5,
							TXPower:  3,
							Redundancy: lorawan.Redundancy{
								NbRep: 2,
							},
						},
						LinkADRAnsPayload: lorawan.LinkADRAnsPayload{
							ChannelMaskACK: true,
							DataRateACK:    true,
							PowerACK:       true,
						},
						ExpectedDeviceSession: storage.DeviceSession{
							EnabledChannels: []int{0, 1, 2},
							TXPowerIndex:    3,
							NbTrans:         2,
							DR:              5,
						},
					},
					{
						Name: "pending request and negative DR ack decrements the max allowed data-rate",
						DeviceSession: storage.DeviceSession{
							EnabledChannels: []int{0, 1},
						},
						LinkADRReqPayload: &lorawan.LinkADRReqPayload{
							ChMask:   lorawan.ChMask{true, true, true},
							DataRate: 5,
							TXPower:  3,
							Redundancy: lorawan.Redundancy{
								NbRep: 2,
							},
						},
						LinkADRAnsPayload: lorawan.LinkADRAnsPayload{
							ChannelMaskACK: true,
							DataRateACK:    false,
							PowerACK:       true,
						},
						ExpectedDeviceSession: storage.DeviceSession{
							EnabledChannels: []int{0, 1},
							MaxSupportedDR:  4,
						},
					},
					{
						Name: "pending request and negative tx-power ack decrements the max allowed tx-power index",
						DeviceSession: storage.DeviceSession{
							EnabledChannels: []int{0, 1},
						},
						LinkADRReqPayload: &lorawan.LinkADRReqPayload{
							ChMask:   lorawan.ChMask{true, true, true},
							DataRate: 5,
							TXPower:  3,
							Redundancy: lorawan.Redundancy{
								NbRep: 2,
							},
						},
						LinkADRAnsPayload: lorawan.LinkADRAnsPayload{
							ChannelMaskACK: true,
							DataRateACK:    true,
							PowerACK:       false,
						},
						ExpectedDeviceSession: storage.DeviceSession{
							EnabledChannels:          []int{0, 1},
							MaxSupportedTXPowerIndex: 2,
						},
					},
					{
						Name: "pending request and negative tx-power ack on tx-power 0 sets tx-power to 1",
						DeviceSession: storage.DeviceSession{
							EnabledChannels: []int{0, 1},
						},
						LinkADRReqPayload: &lorawan.LinkADRReqPayload{
							ChMask:   lorawan.ChMask{true, true, true},
							DataRate: 5,
							TXPower:  0,
							Redundancy: lorawan.Redundancy{
								NbRep: 2,
							},
						},
						LinkADRAnsPayload: lorawan.LinkADRAnsPayload{
							ChannelMaskACK: true,
							DataRateACK:    true,
							PowerACK:       false,
						},
						ExpectedDeviceSession: storage.DeviceSession{
							EnabledChannels: []int{0, 1},
							TXPowerIndex:    1,
						},
					},
					{
						Name: "nothing pending and positive ACK returns an error",
						DeviceSession: storage.DeviceSession{
							EnabledChannels: []int{0, 1},
						},
						LinkADRAnsPayload: lorawan.LinkADRAnsPayload{
							ChannelMaskACK: true,
							DataRateACK:    true,
							PowerACK:       true,
						},
						ExpectedError: ErrDoesNotExist,
						ExpectedDeviceSession: storage.DeviceSession{
							EnabledChannels: []int{0, 1},
						},
					},
				}

				for i, tst := range testTable {
					Convey(fmt.Sprintf("Testing: %s [%d]", tst.Name, i), func() {
						var pending *Block

						if tst.LinkADRReqPayload != nil {
							pending = &Block{
								CID: lorawan.LinkADRReq,
								MACCommands: []lorawan.MACCommand{
									lorawan.MACCommand{
										CID:     lorawan.LinkADRReq,
										Payload: tst.LinkADRReqPayload,
									},
								},
							}
						}

						answer := Block{
							CID: lorawan.LinkADRAns,
							MACCommands: MACCommands{
								lorawan.MACCommand{
									CID:     lorawan.LinkADRAns,
									Payload: &tst.LinkADRAnsPayload,
								},
							},
						}

						err := Handle(&tst.DeviceSession, answer, pending, nil)
						Convey("Then the expected error (or nil) was returned", func() {
							So(err, ShouldResemble, tst.ExpectedError)
						})

						Convey("Then the device-session was updated as expected", func() {
							So(tst.ExpectedDeviceSession, ShouldResemble, tst.DeviceSession)
						})
					})
				}
			})
		})
	})
}
