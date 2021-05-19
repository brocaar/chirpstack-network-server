package uplink

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/brocaar/chirpstack-api/go/v3/common"
	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/brocaar/chirpstack-api/go/v3/nc"
	"github.com/brocaar/chirpstack-network-server/v3/internal/backend/controller"
	"github.com/brocaar/chirpstack-network-server/v3/internal/band"
	"github.com/brocaar/chirpstack-network-server/v3/internal/helpers"
	"github.com/brocaar/chirpstack-network-server/v3/internal/models"
	"github.com/brocaar/chirpstack-network-server/v3/internal/storage"
	"github.com/brocaar/chirpstack-network-server/v3/internal/test"
	"github.com/brocaar/lorawan"
)

type CollectTestSuite struct {
	suite.Suite
}

func (ts *CollectTestSuite) SetupSuite() {
	assert := require.New(ts.T())
	conf := test.GetConfig()
	conf.NetworkServer.DeduplicationDelay = time.Millisecond * 500

	assert.NoError(storage.Setup(conf))
	assert.NoError(Setup(conf))
}

func (ts *CollectTestSuite) TestHandleRejectedUplinkFrameSet() {
	testController := test.NewNetworkControllerClient()
	assert := require.New(ts.T())
	controller.SetClient(testController)

	phy := lorawan.PHYPayload{
		MHDR: lorawan.MHDR{
			MType: lorawan.UnconfirmedDataUp,
			Major: lorawan.LoRaWANR1,
		},
		MIC:        [4]byte{1, 2, 3, 4},
		MACPayload: &lorawan.MACPayload{},
	}
	b, err := phy.MarshalBinary()
	assert.NoError(err)

	uplinkFrame := gw.UplinkFrame{
		PhyPayload: b,
		TxInfo: &gw.UplinkTXInfo{
			Frequency:  868100000,
			Modulation: common.Modulation_LORA,
			ModulationInfo: &gw.UplinkTXInfo_LoraModulationInfo{
				LoraModulationInfo: &gw.LoRaModulationInfo{
					Bandwidth:       125,
					SpreadingFactor: 7,
					CodeRate:        "3/4",
				},
			},
		},
		RxInfo: &gw.UplinkRXInfo{
			GatewayId: []byte{1, 2, 3, 4, 5, 6, 7, 8},
			Location:  &common.Location{},
		},
	}

	assert.Equal(storage.ErrDoesNotExist, errors.Cause(collectUplinkFrames(context.Background(), uplinkFrame)))

	assert.Equal(nc.HandleRejectedUplinkFrameSetRequest{
		FrameSet: &gw.UplinkFrameSet{
			PhyPayload: uplinkFrame.PhyPayload,
			TxInfo:     uplinkFrame.TxInfo,
			RxInfo:     []*gw.UplinkRXInfo{uplinkFrame.RxInfo},
		},
	}, <-testController.HandleRejectedUplinkFrameSetChan)
}

func (ts *CollectTestSuite) TestDeduplication() {
	testTable := []struct {
		Name       string
		PHYPayload lorawan.PHYPayload
		Gateways   []lorawan.EUI64
		Count      int
	}{
		{
			"single item expected",
			lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MIC:        [4]byte{1, 2, 3, 4},
				MACPayload: &lorawan.MACPayload{},
			},
			[]lorawan.EUI64{
				{1, 1, 1, 1, 1, 1, 1, 1},
			},
			1,
		}, {
			"two items expected",
			lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MIC:        [4]byte{2, 2, 3, 4},
				MACPayload: &lorawan.MACPayload{},
			},
			[]lorawan.EUI64{
				{2, 1, 1, 1, 1, 1, 1, 1},
				{2, 2, 2, 2, 2, 2, 2, 2},
			},
			2,
		}, {
			"two items expected (three collected)",
			lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MIC:        [4]byte{3, 2, 3, 4},
				MACPayload: &lorawan.MACPayload{},
			},
			[]lorawan.EUI64{
				{3, 1, 1, 1, 1, 1, 1, 1},
				{3, 2, 2, 2, 2, 2, 2, 2},
				{3, 2, 2, 2, 2, 2, 2, 2},
			},
			2,
		},
	}

	for _, tst := range testTable {
		ts.T().Run(tst.Name, func(t *testing.T) {
			assert := require.New(t)
			storage.RedisClient().FlushAll(context.Background())

			var received int
			var called int

			cb := func(packet models.RXPacket) error {
				called = called + 1
				received = len(packet.RXInfoSet)
				return nil
			}

			var wg sync.WaitGroup
			for i := range tst.Gateways {
				g := tst.Gateways[i]
				phyB, err := tst.PHYPayload.MarshalBinary()
				assert.NoError(err)

				wg.Add(1)
				packet := gw.UplinkFrame{
					RxInfo: &gw.UplinkRXInfo{
						GatewayId: g[:],
					},
					TxInfo:     &gw.UplinkTXInfo{},
					PhyPayload: phyB,
				}
				assert.NoError(helpers.SetUplinkTXInfoDataRate(packet.TxInfo, 0, band.Band()))

				go func(packet gw.UplinkFrame) {
					assert.NoError(collectAndCallOnce(packet, cb))
					wg.Done()
				}(packet)
			}
			wg.Wait()

			assert.Equal(1, called)
			assert.Equal(tst.Count, received)
		})
	}
}

func TestCollect(t *testing.T) {
	suite.Run(t, new(CollectTestSuite))
}
