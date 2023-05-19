//go:build integration
// +build integration

package azureiothub

import (
	"testing"

	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/brocaar/chirpstack-network-server/v3/internal/backend/gateway"
	"github.com/brocaar/chirpstack-network-server/v3/internal/test"
	"github.com/brocaar/lorawan"
	"github.com/joho/godotenv"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type BackendTestSuite struct {
	suite.Suite

	backend   gateway.Gateway
	gatewayID lorawan.EUI64
}

func (ts *BackendTestSuite) SetupSuite() {
	var err error
	godotenv.Load("../../../../.env")
	assert := require.New(ts.T())
	conf := test.GetConfig()

	ts.gatewayID = lorawan.EUI64{0x01, 0x02, 0x03, 0x043, 0x05, 0x06, 0x07, 0x08}

	ts.backend, err = NewBackend(conf)
	assert.NoError(err)
}

func (ts *BackendTestSuite) TestDownlinkCommand() {
	assert := require.New(ts.T())

	pl := gw.DownlinkFrame{
		GatewayId: ts.gatewayID[:],
		Items: []*gw.DownlinkFrameItem{
			{
				PhyPayload: []byte{0x01, 0x02, 0x03, 0x04},
				TxInfo:     &gw.DownlinkTXInfo{},
			},
		},
	}

	assert.NoError(ts.backend.SendTXPacket(pl))
}

func TestBackend(t *testing.T) {
	suite.Run(t, new(BackendTestSuite))
}
