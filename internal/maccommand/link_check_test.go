package maccommand

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/helpers"
	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/loraserver/internal/test"
	"github.com/brocaar/lorawan"
)

type LinkCheckTestSuite struct {
	suite.Suite
	test.DatabaseTestSuiteBase
}

func (ts *LinkCheckTestSuite) TestLinkCheckReq() {
	assert := require.New(ts.T())

	ds := storage.DeviceSession{
		DevEUI:                [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
		EnabledUplinkChannels: []int{0, 1},
	}
	assert.NoError(storage.SaveDeviceSession(ts.RedisPool(), ds))

	block := storage.MACCommandBlock{
		CID: lorawan.LinkCheckReq,
		MACCommands: storage.MACCommands{
			lorawan.MACCommand{
				CID: lorawan.LinkCheckReq,
			},
		},
	}

	rxPacket := models.RXPacket{
		TXInfo: &gw.UplinkTXInfo{},
		RXInfoSet: []*gw.UplinkRXInfo{
			{
				LoraSnr: 5,
			},
		},
	}

	assert.NoError(helpers.SetUplinkTXInfoDataRate(rxPacket.TXInfo, 2, config.C.NetworkServer.Band.Band))

	resp, err := Handle(&ds, storage.DeviceProfile{}, storage.ServiceProfile{}, nil, block, nil, rxPacket)
	assert.NoError(err)

	assert.Len(resp, 1)
	assert.Equal(storage.MACCommandBlock{
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
	}, resp[0])
}

func TestLinkCheck(t *testing.T) {
	suite.Run(t, new(LinkCheckTestSuite))
}
