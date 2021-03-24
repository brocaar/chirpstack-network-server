package maccommand

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/brocaar/chirpstack-network-server/internal/models"
	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/brocaar/chirpstack-network-server/internal/test"
	"github.com/brocaar/lorawan"
)

type DeviceModeIndTestSuite struct {
	suite.Suite

	device *storage.Device
}

func (ts *DeviceModeIndTestSuite) SetupSuite() {
	assert := require.New(ts.T())
	conf := test.GetConfig()
	assert.NoError(storage.Setup(conf))

	assert.NoError(storage.MigrateDown(storage.DB().DB))
	assert.NoError(storage.MigrateUp(storage.DB().DB))

	rp := storage.RoutingProfile{}
	assert.NoError(storage.CreateRoutingProfile(context.Background(), storage.DB(), &rp))

	sp := storage.ServiceProfile{}
	assert.NoError(storage.CreateServiceProfile(context.Background(), storage.DB(), &sp))

	dp := storage.DeviceProfile{}
	assert.NoError(storage.CreateDeviceProfile(context.Background(), storage.DB(), &dp))

	ts.device = &storage.Device{
		RoutingProfileID: rp.ID,
		ServiceProfileID: sp.ID,
		DeviceProfileID:  dp.ID,
		DevEUI:           lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
		Mode:             storage.DeviceModeA,
	}
	assert.NoError(storage.CreateDevice(context.Background(), storage.DB(), ts.device))
}

func (ts *DeviceModeIndTestSuite) TestDeviceModeInd() {
	tests := []struct {
		Name                    string
		ReceivedMACCommandBlock storage.MACCommandBlock
		ExpectedDeviceMode      storage.DeviceMode
		ExpectedMACCommandBlock storage.MACCommandBlock
		ExpectedError           error
	}{
		{
			Name: "Class-C",
			ReceivedMACCommandBlock: storage.MACCommandBlock{
				CID: lorawan.DeviceModeInd,
				MACCommands: storage.MACCommands{
					{
						CID: lorawan.DeviceModeInd,
						Payload: &lorawan.DeviceModeIndPayload{
							Class: lorawan.DeviceModeClassC,
						},
					},
				},
			},
			ExpectedDeviceMode: storage.DeviceModeC,
			ExpectedMACCommandBlock: storage.MACCommandBlock{
				CID: lorawan.DeviceModeConf,
				MACCommands: storage.MACCommands{
					{
						CID: lorawan.DeviceModeConf,
						Payload: &lorawan.DeviceModeConfPayload{
							Class: lorawan.DeviceModeClassC,
						},
					},
				},
			},
		},
		{
			Name: "Class-A",
			ReceivedMACCommandBlock: storage.MACCommandBlock{
				CID: lorawan.DeviceModeInd,
				MACCommands: storage.MACCommands{
					{
						CID: lorawan.DeviceModeInd,
						Payload: &lorawan.DeviceModeIndPayload{
							Class: lorawan.DeviceModeClassA,
						},
					},
				},
			},
			ExpectedDeviceMode: storage.DeviceModeA,
			ExpectedMACCommandBlock: storage.MACCommandBlock{
				CID: lorawan.DeviceModeConf,
				MACCommands: storage.MACCommands{
					{
						CID: lorawan.DeviceModeConf,
						Payload: &lorawan.DeviceModeConfPayload{
							Class: lorawan.DeviceModeClassA,
						},
					},
				},
			},
		},
		{
			Name: "Error",
			ReceivedMACCommandBlock: storage.MACCommandBlock{
				CID: lorawan.DeviceModeInd,
				MACCommands: storage.MACCommands{
					{
						CID: lorawan.DeviceModeInd,
						Payload: &lorawan.DeviceModeIndPayload{
							Class: lorawan.DeviceModeRFU,
						},
					},
				},
			},
			ExpectedError: errors.New("unexpected device mode: DeviceModeRFU"),
		},
	}

	for _, tst := range tests {
		ts.T().Run(tst.Name, func(t *testing.T) {
			assert := require.New(t)
			ctx := context.Background()

			block, err := Handle(ctx, &storage.DeviceSession{DevEUI: ts.device.DevEUI}, storage.DeviceProfile{}, storage.ServiceProfile{}, nil, tst.ReceivedMACCommandBlock, nil, models.RXPacket{})
			if tst.ExpectedError != nil {
				assert.Equal(tst.ExpectedError.Error(), err.Error())
				return
			}

			assert.NoError(err)
			assert.Len(block, 1)
			assert.Equal(tst.ExpectedMACCommandBlock, block[0])

			d, err := storage.GetDevice(ctx, storage.DB(), ts.device.DevEUI, false)
			assert.NoError(err)
			assert.Equal(tst.ExpectedDeviceMode, d.Mode)
		})
	}
}

func TestDeviceModeInd(t *testing.T) {
	suite.Run(t, new(DeviceModeIndTestSuite))
}
