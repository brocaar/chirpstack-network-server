package maccommand

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/brocaar/lorawan"
)

func TestTXParamSetup(t *testing.T) {
	t.Run("RequestTXParamSetup", func(t *testing.T) {
		assert := require.New(t)

		assert.Equal(storage.MACCommandBlock{
			CID: lorawan.TXParamSetupReq,
			MACCommands: []lorawan.MACCommand{
				{
					CID: lorawan.TXParamSetupReq,
					Payload: &lorawan.TXParamSetupReqPayload{
						DownlinkDwelltime: lorawan.DwellTimeNoLimit,
						UplinkDwellTime:   lorawan.DwellTime400ms,
						MaxEIRP:           10,
					},
				},
			},
		}, RequestTXParamSetup(true, false, 10))
	})

	t.Run("handleTXParamSetupAns", func(t *testing.T) {
		tests := []struct {
			Name                  string
			DeviceSession         storage.DeviceSession
			PendingBlock          *storage.MACCommandBlock
			ExpectedDeviceSession storage.DeviceSession
			ExpectedError         error
		}{
			{
				Name: "request acked",
				DeviceSession: storage.DeviceSession{
					UplinkDwellTime400ms:   false,
					DownlinkDwellTime400ms: true,
					UplinkMaxEIRPIndex:     10,
				},
				PendingBlock: &storage.MACCommandBlock{
					CID: lorawan.TXParamSetupReq,
					MACCommands: []lorawan.MACCommand{
						{
							CID: lorawan.TXParamSetupReq,
							Payload: &lorawan.TXParamSetupReqPayload{
								UplinkDwellTime:   lorawan.DwellTime400ms,
								DownlinkDwelltime: lorawan.DwellTimeNoLimit,
								MaxEIRP:           14,
							},
						},
					},
				},
				ExpectedDeviceSession: storage.DeviceSession{
					UplinkDwellTime400ms:   true,
					DownlinkDwellTime400ms: false,
					UplinkMaxEIRPIndex:     14,
				},
			},
			{
				Name: "pending missing",
				DeviceSession: storage.DeviceSession{
					UplinkDwellTime400ms:   false,
					DownlinkDwellTime400ms: true,
					UplinkMaxEIRPIndex:     10,
				},
				ExpectedDeviceSession: storage.DeviceSession{
					UplinkDwellTime400ms:   false,
					DownlinkDwellTime400ms: true,
					UplinkMaxEIRPIndex:     10,
				},
				ExpectedError: errors.New("expected pending mac-command"),
			},
		}

		for _, tst := range tests {
			t.Run(tst.Name, func(t *testing.T) {
				assert := require.New(t)

				ret, err := handleTXParamSetupAns(context.Background(), &tst.DeviceSession, storage.MACCommandBlock{}, tst.PendingBlock)
				assert.Nil(ret)
				assert.Equal(tst.ExpectedDeviceSession, tst.DeviceSession)
				if err != nil {
					assert.Equal(tst.ExpectedError.Error(), err.Error())
				} else {
					assert.Nil(tst.ExpectedError)
				}
			})
		}
	})
}
