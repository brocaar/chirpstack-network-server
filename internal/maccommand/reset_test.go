package maccommand

import (
	"context"
	"fmt"
	"testing"

	"github.com/brocaar/lorawan/band"

	"github.com/brocaar/chirpstack-network-server/v3/internal/storage"
	"github.com/brocaar/lorawan"
	. "github.com/smartystreets/goconvey/convey"
)

func TestReset(t *testing.T) {
	Convey("Given a set of tests", t, func() {
		tests := []struct {
			Name                    string
			DevMinorVersion         uint8
			DeviceSession           storage.DeviceSession
			DeviceProfile           storage.DeviceProfile
			ExpectedMACCommandBlock storage.MACCommandBlock
			ExpectedDeviceSession   storage.DeviceSession
		}{
			{
				Name:            "LoRaWAN 1.1 device",
				DevMinorVersion: 1,
				DeviceProfile: storage.DeviceProfile{
					RXDelay1:           1,
					RXDROffset1:        0,
					RXDataRate2:        0,
					RXFreq2:            868300000,
					FactoryPresetFreqs: []uint32{868100000, 868300000, 868500000},
					PingSlotDR:         2,
					PingSlotFreq:       868100000,
					PingSlotPeriod:     1,
				},
				DeviceSession: storage.DeviceSession{
					TXPowerIndex:             3,
					MinSupportedTXPowerIndex: 1,
					MaxSupportedTXPowerIndex: 5,
					ExtraUplinkChannels: map[int]band.Channel{
						3: band.Channel{},
					},
					RXDelay:               3,
					RX1DROffset:           1,
					RX2DR:                 5,
					RX2Frequency:          868900000,
					EnabledUplinkChannels: []int{0, 1},
					PingSlotDR:            3,
					PingSlotFrequency:     868100000,
					NbTrans:               3,
				},
				ExpectedMACCommandBlock: storage.MACCommandBlock{
					CID: lorawan.ResetConf,
					MACCommands: []lorawan.MACCommand{
						{
							CID: lorawan.ResetConf,
							Payload: &lorawan.ResetConfPayload{
								ServLoRaWANVersion: lorawan.Version{
									Minor: 1,
								},
							},
						},
					},
				},
				ExpectedDeviceSession: storage.DeviceSession{
					RXDelay:                  1,
					RX1DROffset:              0,
					RX2DR:                    0,
					RX2Frequency:             868300000,
					TXPowerIndex:             0,
					DR:                       0,
					MinSupportedTXPowerIndex: 0,
					MaxSupportedTXPowerIndex: 0,
					NbTrans:                  1,
					EnabledUplinkChannels:    []int{0, 1, 2},
					ChannelFrequencies:       []uint32{868100000, 868300000, 868500000},
					PingSlotNb:               4096,
					PingSlotDR:               2,
					PingSlotFrequency:        868100000,
					ExtraUplinkChannels:      map[int]band.Channel{},
				},
			},
		}

		for i, test := range tests {
			Convey(fmt.Sprintf("Testing: %s [%d]", test.Name, i), func() {
				req := storage.MACCommandBlock{
					CID: lorawan.ResetInd,
					MACCommands: []lorawan.MACCommand{
						{
							CID: lorawan.ResetInd,
							Payload: &lorawan.ResetIndPayload{
								DevLoRaWANVersion: lorawan.Version{
									Minor: test.DevMinorVersion,
								},
							},
						},
					},
				}

				out, err := handleResetInd(context.Background(), &test.DeviceSession, test.DeviceProfile, req)
				So(err, ShouldBeNil)
				So(out, ShouldHaveLength, 1)
				So(out[0], ShouldResemble, test.ExpectedMACCommandBlock)

				So(test.DeviceSession, ShouldResemble, test.ExpectedDeviceSession)
			})
		}
	})
}
