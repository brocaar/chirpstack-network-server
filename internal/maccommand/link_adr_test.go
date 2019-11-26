package maccommand

import (
	"context"
	"testing"

	"github.com/brocaar/chirpstack-network-server/internal/band"
	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/brocaar/chirpstack-network-server/internal/test"
	"github.com/brocaar/lorawan"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestLinkADR(t *testing.T) {
	assert := require.New(t)

	conf := test.GetConfig()
	assert.NoError(band.Setup(conf))

	t.Run("handleLinkADRAns", func(t *testing.T) {
		tests := []struct {
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
					MACCommandErrorCount: map[lorawan.CID]int{
						lorawan.LinkADRReq: 1,
					},
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
					MACCommandErrorCount:  map[lorawan.CID]int{},
				},
			},
			{
				Name: "pending request and negative tx-power ack decrements the max allowed tx-power index",
				DeviceSession: storage.DeviceSession{
					EnabledUplinkChannels: []int{0, 1},
					MACCommandErrorCount:  map[lorawan.CID]int{},
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
					MACCommandErrorCount: map[lorawan.CID]int{
						lorawan.LinkADRAns: 1,
					},
				},
			},
			{
				Name: "pending request and negative tx-power ack on tx-power 0 sets (min) tx-power to 1",
				DeviceSession: storage.DeviceSession{
					EnabledUplinkChannels: []int{0, 1},
					MACCommandErrorCount:  map[lorawan.CID]int{},
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
					MACCommandErrorCount: map[lorawan.CID]int{
						lorawan.LinkADRAns: 1,
					},
				},
			},
			{
				Name: "nothing pending and positive ACK returns an error",
				DeviceSession: storage.DeviceSession{
					EnabledUplinkChannels: []int{0, 1},
					MACCommandErrorCount:  map[lorawan.CID]int{},
				},
				LinkADRAnsPayload: lorawan.LinkADRAnsPayload{
					ChannelMaskACK: true,
					DataRateACK:    true,
					PowerACK:       true,
				},
				ExpectedError: errors.New("expected pending mac-command"),
				ExpectedDeviceSession: storage.DeviceSession{
					EnabledUplinkChannels: []int{0, 1},
					MACCommandErrorCount:  map[lorawan.CID]int{},
				},
			},
		}

		for _, tst := range tests {
			t.Run(tst.Name, func(t *testing.T) {
				assert := require.New(t)

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
				resp, err := handleLinkADRAns(context.Background(), &tst.DeviceSession, answer, pending)
				if tst.ExpectedError != nil {
					assert.Equal(tst.ExpectedError.Error(), err.Error())
					return
				}
				assert.Nil(err)
				assert.Nil(resp)
				assert.Equal(tst.ExpectedDeviceSession, tst.DeviceSession)
			})
		}
	})
}
