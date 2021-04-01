package maccommand

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/brocaar/chirpstack-network-server/v3/internal/storage"
	"github.com/brocaar/lorawan"
)

func TestRXParamSetup(t *testing.T) {
	t.Run("RequestRXParamSetup", func(t *testing.T) {
		assert := require.New(t)
		assert.Equal(storage.MACCommandBlock{
			CID: lorawan.RXParamSetupReq,
			MACCommands: []lorawan.MACCommand{
				{
					CID: lorawan.RXParamSetupReq,
					Payload: &lorawan.RXParamSetupReqPayload{
						Frequency: 868700000,
						DLSettings: lorawan.DLSettings{
							RX2DataRate: 5,
							RX1DROffset: 2,
						},
					},
				},
			},
		}, RequestRXParamSetup(2, 868700000, 5))
	})

	t.Run("handleRXParamSetup", func(t *testing.T) {
		tests := []struct {
			Name                    string
			DeviceSession           storage.DeviceSession
			ReceivedMACCommandBlock storage.MACCommandBlock
			PendingMACCommandBlock  *storage.MACCommandBlock
			ExpectedDeviceSession   storage.DeviceSession
			ExpectedError           error
		}{
			{
				Name: "rx param setup ack",
				DeviceSession: storage.DeviceSession{
					RX2Frequency: 868100000,
					RX2DR:        0,
					RX1DROffset:  1,
					MACCommandErrorCount: map[lorawan.CID]int{
						lorawan.RXParamSetupAns: 1,
					},
				},
				ReceivedMACCommandBlock: storage.MACCommandBlock{
					CID: lorawan.RXParamSetupAns,
					MACCommands: storage.MACCommands{
						{
							CID: lorawan.RXParamSetupAns,
							Payload: &lorawan.RXParamSetupAnsPayload{
								ChannelACK:     true,
								RX2DataRateACK: true,
								RX1DROffsetACK: true,
							},
						},
					},
				},
				PendingMACCommandBlock: &storage.MACCommandBlock{
					CID: lorawan.RXParamSetupReq,
					MACCommands: storage.MACCommands{
						{
							CID: lorawan.RXParamSetupReq,
							Payload: &lorawan.RXParamSetupReqPayload{
								Frequency: 868700000,
								DLSettings: lorawan.DLSettings{
									RX2DataRate: 5,
									RX1DROffset: 2,
								},
							},
						},
					},
				},
				ExpectedDeviceSession: storage.DeviceSession{
					RX2Frequency:         868700000,
					RX2DR:                5,
					RX1DROffset:          2,
					MACCommandErrorCount: map[lorawan.CID]int{},
				},
			},
			{
				Name: "rx param setup nack",
				DeviceSession: storage.DeviceSession{
					RX2Frequency: 868100000,
					RX2DR:        0,
					RX1DROffset:  1,
					MACCommandErrorCount: map[lorawan.CID]int{
						lorawan.RXParamSetupAns: 1,
					},
				},
				ReceivedMACCommandBlock: storage.MACCommandBlock{
					CID: lorawan.RXParamSetupAns,
					MACCommands: storage.MACCommands{
						{
							CID: lorawan.RXParamSetupAns,
							Payload: &lorawan.RXParamSetupAnsPayload{
								ChannelACK:     true,
								RX2DataRateACK: false,
								RX1DROffsetACK: true,
							},
						},
					},
				},
				PendingMACCommandBlock: &storage.MACCommandBlock{
					CID: lorawan.RXParamSetupReq,
					MACCommands: storage.MACCommands{
						{
							CID: lorawan.RXParamSetupReq,
							Payload: &lorawan.RXParamSetupReqPayload{
								Frequency: 868700000,
								DLSettings: lorawan.DLSettings{
									RX2DataRate: 5,
									RX1DROffset: 2,
								},
							},
						},
					},
				},
				ExpectedDeviceSession: storage.DeviceSession{
					RX2Frequency: 868100000,
					RX2DR:        0,
					RX1DROffset:  1,
					MACCommandErrorCount: map[lorawan.CID]int{
						lorawan.RXParamSetupAns: 2,
					},
				},
			},
		}

		for _, tst := range tests {
			t.Run(tst.Name, func(t *testing.T) {
				assert := require.New(t)
				ans, err := handleRXParamSetupAns(context.Background(), &tst.DeviceSession, tst.ReceivedMACCommandBlock, tst.PendingMACCommandBlock)
				assert.NoError(err)
				assert.Nil(ans)

				assert.Equal(tst.ExpectedDeviceSession, tst.DeviceSession)
			})

		}
	})
}
