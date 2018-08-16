package maccommand

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/loraserver/internal/test"
	"github.com/brocaar/lorawan"
)

type PingSlotInfoTestSuite struct {
	suite.Suite
	test.DatabaseTestSuiteBase
}

func (ts *PingSlotInfoTestSuite) TestPingSlotInfoReq() {
	assert := require.New(ts.T())

	ds := storage.DeviceSession{
		DevEUI:                [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
		EnabledUplinkChannels: []int{0, 1},
	}
	assert.NoError(storage.SaveDeviceSession(ts.RedisPool(), ds))

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
	assert.NoError(err)

	assert.Equal(16, ds.PingSlotNb)

	assert.Len(resp, 1)
	assert.Equal(storage.MACCommandBlock{
		CID: lorawan.PingSlotInfoAns,
		MACCommands: []lorawan.MACCommand{
			{
				CID: lorawan.PingSlotInfoAns,
			},
		},
	}, resp[0])
}

func TestPingSlotInfo(t *testing.T) {
	suite.Run(t, new(PingSlotInfoTestSuite))
}
