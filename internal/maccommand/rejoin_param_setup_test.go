package maccommand

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/brocaar/lorawan"
)

func TestRejoinParamSetup(t *testing.T) {
	t.Run("RejoinParamSetupReq", func(t *testing.T) {
		assert := require.New(t)

		assert.Equal(storage.MACCommandBlock{
			CID: lorawan.RejoinParamSetupReq,
			MACCommands: []lorawan.MACCommand{
				{
					CID: lorawan.RejoinParamSetupReq,
					Payload: &lorawan.RejoinParamSetupReqPayload{
						MaxTimeN:  5,
						MaxCountN: 10,
					},
				},
			},
		}, RequestRejoinParamSetup(5, 10))
	})

	t.Run("handleRejoinParamSetupAns", func(t *testing.T) {
		tests := []struct {
			Name                    string
			DeviceSession           storage.DeviceSession
			ReceivedMACCommandBlock storage.MACCommandBlock
			PendingMACCommandBlock  *storage.MACCommandBlock
			ExpectedDeviceSession   storage.DeviceSession
			ExpectedError           error
		}{
			{
				Name: "acknowledged with time ok",
				DeviceSession: storage.DeviceSession{
					RejoinRequestMaxCountN: 1,
					RejoinRequestMaxTimeN:  2,
				},
				ReceivedMACCommandBlock: storage.MACCommandBlock{
					CID: lorawan.RejoinParamSetupAns,
					MACCommands: []lorawan.MACCommand{
						{
							CID: lorawan.RejoinParamSetupAns,
							Payload: &lorawan.RejoinParamSetupAnsPayload{
								TimeOK: true,
							},
						},
					},
				},
				PendingMACCommandBlock: &storage.MACCommandBlock{
					CID: lorawan.RejoinParamSetupReq,
					MACCommands: []lorawan.MACCommand{
						{
							CID: lorawan.RejoinParamSetupReq,
							Payload: &lorawan.RejoinParamSetupReqPayload{
								MaxCountN: 10,
								MaxTimeN:  5,
							},
						},
					},
				},
				ExpectedDeviceSession: storage.DeviceSession{
					RejoinRequestEnabled:   true,
					RejoinRequestMaxCountN: 10,
					RejoinRequestMaxTimeN:  5,
				},
			},
			{
				// this still is handled as an ACK, only with a printed warning.
				Name: "acknowledged with time not ok",
				DeviceSession: storage.DeviceSession{
					RejoinRequestMaxCountN: 1,
					RejoinRequestMaxTimeN:  2,
				},
				ReceivedMACCommandBlock: storage.MACCommandBlock{
					CID: lorawan.RejoinParamSetupAns,
					MACCommands: []lorawan.MACCommand{
						{
							CID: lorawan.RejoinParamSetupAns,
							Payload: &lorawan.RejoinParamSetupAnsPayload{
								TimeOK: false,
							},
						},
					},
				},
				PendingMACCommandBlock: &storage.MACCommandBlock{
					CID: lorawan.RejoinParamSetupReq,
					MACCommands: []lorawan.MACCommand{
						{
							CID: lorawan.RejoinParamSetupReq,
							Payload: &lorawan.RejoinParamSetupReqPayload{
								MaxCountN: 10,
								MaxTimeN:  5,
							},
						},
					},
				},
				ExpectedDeviceSession: storage.DeviceSession{
					RejoinRequestEnabled:   true,
					RejoinRequestMaxCountN: 10,
					RejoinRequestMaxTimeN:  5,
				},
			},
			{
				Name: "acknowledged, but nothing pending",
				DeviceSession: storage.DeviceSession{
					RejoinRequestMaxCountN: 1,
					RejoinRequestMaxTimeN:  2,
				},
				ReceivedMACCommandBlock: storage.MACCommandBlock{
					CID: lorawan.RejoinParamSetupAns,
					MACCommands: []lorawan.MACCommand{
						{
							CID: lorawan.RejoinParamSetupAns,
							Payload: &lorawan.RejoinParamSetupAnsPayload{
								TimeOK: true,
							},
						},
					},
				},
				ExpectedError: errors.New("expected pending mac-command"),
				ExpectedDeviceSession: storage.DeviceSession{
					RejoinRequestMaxCountN: 1,
					RejoinRequestMaxTimeN:  2,
				},
			},
		}

		for _, tst := range tests {
			t.Run(tst.Name, func(t *testing.T) {
				assert := require.New(t)

				ans, err := handleRejoinParamSetupAns(context.Background(), &tst.DeviceSession, tst.ReceivedMACCommandBlock, tst.PendingMACCommandBlock)
				if tst.ExpectedError != nil {
					assert.Equal(tst.ExpectedError.Error(), err.Error())
					return
				}
				assert.NoError(err)
				assert.Nil(ans)
				assert.Equal(tst.ExpectedDeviceSession, tst.DeviceSession)
			})
		}

	})
}
