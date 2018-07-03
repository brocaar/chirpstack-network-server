package maccommand

import (
	"fmt"
	"testing"

	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/loraserver/internal/test"
	"github.com/brocaar/lorawan"
	"github.com/pkg/errors"
	. "github.com/smartystreets/goconvey/convey"
)

func TestHandleUplink(t *testing.T) {
	conf := test.GetConfig()

	Convey("Given a clean Redis database", t, func() {
		config.C.Redis.Pool = common.NewRedisPool(conf.RedisURL)
		test.MustFlushRedis(config.C.Redis.Pool)

		Convey("Given a device-session", func() {
			ds := storage.DeviceSession{
				DevEUI:                [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
				EnabledUplinkChannels: []int{0, 1},
			}
			So(storage.SaveDeviceSession(config.C.Redis.Pool, ds), ShouldBeNil)

			Convey("Test LinkCheckReq", func() {
				dr2, err := config.C.NetworkServer.Band.Band.GetDataRate(2)
				So(err, ShouldBeNil)

				block := storage.MACCommandBlock{
					CID: lorawan.LinkCheckReq,
					MACCommands: storage.MACCommands{
						lorawan.MACCommand{
							CID: lorawan.LinkCheckReq,
						},
					},
				}

				rxPacket := models.RXPacket{
					TXInfo: models.TXInfo{
						DataRate: dr2,
					},
					RXInfoSet: models.RXInfoSet{
						{
							LoRaSNR: 5,
						},
					},
				}

				resp, err := Handle(&ds, storage.DeviceProfile{}, storage.ServiceProfile{}, nil, block, nil, rxPacket)
				So(err, ShouldBeNil)

				Convey("Then the expected response was returned", func() {
					So(resp, ShouldHaveLength, 1)
					So(resp[0], ShouldResemble, storage.MACCommandBlock{
						CID: lorawan.LinkCheckAns,
						MACCommands: storage.MACCommands{
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

			Convey("Test PingSlotInfoReq", func() {
				block := storage.MACCommandBlock{
					CID: lorawan.PingSlotInfoReq,
					MACCommands: []lorawan.MACCommand{
						{
							CID: lorawan.PingSlotInfoReq,
							Payload: &lorawan.PingSlotInfoReqPayload{
								Periodicity: 3,
							},
						},
					},
				}

				resp, err := Handle(&ds, storage.DeviceProfile{}, storage.ServiceProfile{}, nil, block, nil, models.RXPacket{})
				So(err, ShouldBeNil)

				Convey("Then the ClassB PingNb has been set", func() {
					So(ds.PingSlotNb, ShouldEqual, 16)
				})

				Convey("Then the expected response was returned", func() {
					So(resp, ShouldHaveLength, 1)
					So(resp[0], ShouldResemble, storage.MACCommandBlock{
						CID: lorawan.PingSlotInfoAns,
						MACCommands: []lorawan.MACCommand{
							{
								CID: lorawan.PingSlotInfoAns,
							},
						},
					})
				})
			})
		})
	})
}

func TestHandleDownlink(t *testing.T) {
	conf := test.GetConfig()

	Convey("Given a clean Redis database", t, func() {
		config.C.Redis.Pool = common.NewRedisPool(conf.RedisURL)
		test.MustFlushRedis(config.C.Redis.Pool)

		Convey("Given a device-session", func() {
			ds := storage.DeviceSession{
				DevEUI:                [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
				EnabledUplinkChannels: []int{0, 1},
			}
			So(storage.SaveDeviceSession(config.C.Redis.Pool, ds), ShouldBeNil)

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
							EnabledUplinkChannels: []int{0, 1},
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
							EnabledUplinkChannels: []int{0, 1, 2},
							TXPowerIndex:          3,
							NbTrans:               2,
							DR:                    5,
						},
					},
					{
						Name: "pending request and negative DR ack decrements the max allowed data-rate",
						DeviceSession: storage.DeviceSession{
							EnabledUplinkChannels: []int{0, 1},
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
							EnabledUplinkChannels: []int{0, 1},
							MaxSupportedDR:        4,
						},
					},
					{
						Name: "pending request and negative tx-power ack decrements the max allowed tx-power index",
						DeviceSession: storage.DeviceSession{
							EnabledUplinkChannels: []int{0, 1},
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
							EnabledUplinkChannels:    []int{0, 1},
							MaxSupportedTXPowerIndex: 2,
						},
					},
					{
						Name: "pending request and negative tx-power ack on tx-power 0 sets (min) tx-power to 1",
						DeviceSession: storage.DeviceSession{
							EnabledUplinkChannels: []int{0, 1},
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
							EnabledUplinkChannels:    []int{0, 1},
							TXPowerIndex:             1,
							MinSupportedTXPowerIndex: 1,
						},
					},
					{
						Name: "nothing pending and positive ACK returns an error",
						DeviceSession: storage.DeviceSession{
							EnabledUplinkChannels: []int{0, 1},
						},
						LinkADRAnsPayload: lorawan.LinkADRAnsPayload{
							ChannelMaskACK: true,
							DataRateACK:    true,
							PowerACK:       true,
						},
						ExpectedError: errors.New("expected pending mac-command"),
						ExpectedDeviceSession: storage.DeviceSession{
							EnabledUplinkChannels: []int{0, 1},
						},
					},
				}

				for i, tst := range testTable {
					Convey(fmt.Sprintf("Testing: %s [%d]", tst.Name, i), func() {
						var pending *storage.MACCommandBlock

						if tst.LinkADRReqPayload != nil {
							pending = &storage.MACCommandBlock{
								CID: lorawan.LinkADRReq,
								MACCommands: []lorawan.MACCommand{
									lorawan.MACCommand{
										CID:     lorawan.LinkADRReq,
										Payload: tst.LinkADRReqPayload,
									},
								},
							}
						}

						answer := storage.MACCommandBlock{
							CID: lorawan.LinkADRAns,
							MACCommands: storage.MACCommands{
								lorawan.MACCommand{
									CID:     lorawan.LinkADRAns,
									Payload: &tst.LinkADRAnsPayload,
								},
							},
						}

						resp, err := Handle(&tst.DeviceSession, storage.DeviceProfile{}, storage.ServiceProfile{}, nil, answer, pending, models.RXPacket{})
						Convey("Then the expected error (or nil) was returned", func() {
							if err != nil && tst.ExpectedError != nil {
								So(err.Error(), ShouldResemble, tst.ExpectedError.Error())
							} else {
								So(err, ShouldResemble, tst.ExpectedError)
							}
						})
						So(resp, ShouldHaveLength, 0)

						Convey("Then the device-session was updated as expected", func() {
							So(tst.DeviceSession, ShouldResemble, tst.ExpectedDeviceSession)
						})
					})
				}
			})

			Convey("Testing PingSlotChannelAns", func() {
				testTable := []struct {
					Name                  string
					DeviceSession         storage.DeviceSession
					PingSlotChannelReq    *lorawan.PingSlotChannelReqPayload
					PingSlotChannelAns    lorawan.PingSlotChannelAnsPayload
					ExpectedDeviceSession storage.DeviceSession
					ExpectedError         error
				}{
					{
						Name: "pending request and positive ACK updates frequency and data-rate",
						DeviceSession: storage.DeviceSession{
							PingSlotFrequency: 868100000,
							PingSlotDR:        3,
						},
						PingSlotChannelReq: &lorawan.PingSlotChannelReqPayload{
							Frequency: 868300000,
							DR:        4,
						},
						PingSlotChannelAns: lorawan.PingSlotChannelAnsPayload{
							DataRateOK:         true,
							ChannelFrequencyOK: true,
						},
						ExpectedDeviceSession: storage.DeviceSession{
							PingSlotFrequency: 868300000,
							PingSlotDR:        4,
						},
					},
					{
						Name: "pending request and negative ACK does not update",
						DeviceSession: storage.DeviceSession{
							PingSlotFrequency: 868100000,
							PingSlotDR:        3,
						},
						PingSlotChannelReq: &lorawan.PingSlotChannelReqPayload{
							Frequency: 868300000 / 100,
							DR:        4,
						},
						PingSlotChannelAns: lorawan.PingSlotChannelAnsPayload{
							DataRateOK:         false,
							ChannelFrequencyOK: true,
						},
						ExpectedDeviceSession: storage.DeviceSession{
							PingSlotFrequency: 868100000,
							PingSlotDR:        3,
						},
					},
					{
						Name: "no pending request and positive ACK returns an error",
						DeviceSession: storage.DeviceSession{
							PingSlotFrequency: 868100000,
							PingSlotDR:        3,
						},
						PingSlotChannelAns: lorawan.PingSlotChannelAnsPayload{
							DataRateOK:         false,
							ChannelFrequencyOK: true,
						},
						ExpectedError: errors.New("expected pending mac-command"),
						ExpectedDeviceSession: storage.DeviceSession{
							PingSlotFrequency: 868100000,
							PingSlotDR:        3,
						},
					},
				}

				for i, test := range testTable {
					Convey(fmt.Sprintf("Testing: %s [%d]", test.Name, i), func() {
						var pending *storage.MACCommandBlock
						if test.PingSlotChannelReq != nil {
							pending = &storage.MACCommandBlock{
								CID: lorawan.PingSlotChannelReq,
								MACCommands: []lorawan.MACCommand{
									lorawan.MACCommand{
										CID:     lorawan.PingSlotChannelReq,
										Payload: test.PingSlotChannelReq,
									},
								},
							}
						}

						answer := storage.MACCommandBlock{
							CID: lorawan.PingSlotChannelAns,
							MACCommands: []lorawan.MACCommand{
								lorawan.MACCommand{
									CID:     lorawan.PingSlotChannelAns,
									Payload: &test.PingSlotChannelAns,
								},
							},
						}

						_, err := Handle(&test.DeviceSession, storage.DeviceProfile{}, storage.ServiceProfile{}, nil, answer, pending, models.RXPacket{})
						Convey("Then the expected error (or nil) was returned", func() {
							if err != nil && test.ExpectedError != nil {
								So(err.Error(), ShouldEqual, test.ExpectedError.Error())
							} else {
								So(err, ShouldResemble, test.ExpectedError)
							}
						})

						Convey("Then the device-session was updated as expected", func() {
							So(test.ExpectedDeviceSession, ShouldResemble, test.DeviceSession)
						})
					})
				}
			})
		})
	})
}
