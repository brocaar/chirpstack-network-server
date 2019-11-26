package maccommand

import (
	"context"
	"testing"

	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/brocaar/lorawan"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestPingSlotChannel(t *testing.T) {
	t.Run("PingSlotChannelReq", func(t *testing.T) {
		assert := require.New(t)
		assert.Equal(storage.MACCommandBlock{
			CID: lorawan.PingSlotChannelReq,
			MACCommands: []lorawan.MACCommand{
				{
					CID: lorawan.PingSlotChannelReq,
					Payload: &lorawan.PingSlotChannelReqPayload{
						Frequency: 868100000,
						DR:        3,
					},
				},
			},
		}, RequestPingSlotChannel(lorawan.EUI64{}, 3, 868100000))
	})

	t.Run("handlePingSlotChannelAns", func(t *testing.T) {
		tests := []struct {
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
					MACCommandErrorCount: map[lorawan.CID]int{
						lorawan.PingSlotChannelAns: 1,
					},
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
					PingSlotFrequency:    868300000,
					PingSlotDR:           4,
					MACCommandErrorCount: map[lorawan.CID]int{},
				},
			},
			{
				Name: "pending request and negative ACK does not update",
				DeviceSession: storage.DeviceSession{
					PingSlotFrequency:    868100000,
					PingSlotDR:           3,
					MACCommandErrorCount: map[lorawan.CID]int{},
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
					MACCommandErrorCount: map[lorawan.CID]int{
						lorawan.PingSlotChannelAns: 1,
					},
				},
			},
			{
				Name: "no pending request and positive ACK returns an error",
				DeviceSession: storage.DeviceSession{
					PingSlotFrequency:    868100000,
					PingSlotDR:           3,
					MACCommandErrorCount: map[lorawan.CID]int{},
				},
				PingSlotChannelAns: lorawan.PingSlotChannelAnsPayload{
					DataRateOK:         false,
					ChannelFrequencyOK: true,
				},
				ExpectedError: errors.New("expected pending mac-command"),
				ExpectedDeviceSession: storage.DeviceSession{
					PingSlotFrequency:    868100000,
					PingSlotDR:           3,
					MACCommandErrorCount: map[lorawan.CID]int{},
				},
			},
		}

		for _, tst := range tests {
			t.Run(tst.Name, func(t *testing.T) {
				assert := require.New(t)

				var pending *storage.MACCommandBlock
				if tst.PingSlotChannelReq != nil {
					pending = &storage.MACCommandBlock{
						CID: lorawan.PingSlotChannelReq,
						MACCommands: []lorawan.MACCommand{
							lorawan.MACCommand{
								CID:     lorawan.PingSlotChannelReq,
								Payload: tst.PingSlotChannelReq,
							},
						},
					}
				}

				answer := storage.MACCommandBlock{
					CID: lorawan.PingSlotChannelAns,
					MACCommands: []lorawan.MACCommand{
						lorawan.MACCommand{
							CID:     lorawan.PingSlotChannelAns,
							Payload: &tst.PingSlotChannelAns,
						},
					},
				}

				resp, err := handlePingSlotChannelAns(context.Background(), &tst.DeviceSession, answer, pending)
				if tst.ExpectedError != nil {
					assert.Equal(tst.ExpectedError.Error(), err.Error())
					return
				}
				assert.NoError(err)
				assert.Nil(resp)
				assert.Equal(tst.ExpectedDeviceSession, tst.DeviceSession)
			})
		}
	})
}
