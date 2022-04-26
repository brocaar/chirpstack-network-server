package uplink

import (
	"context"
	"encoding/hex"
	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"
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

func TestGetKeyWithFrequencySetToZero(t *testing.T) {
	originalFreq := uint32(868100000)

	uplinkFrame := gw.UplinkFrame{
		TxInfo: &gw.UplinkTXInfo{
			Frequency:  originalFreq,
			Modulation: common.Modulation_LORA,
			ModulationInfo: &gw.UplinkTXInfo_LoraModulationInfo{
				LoraModulationInfo: &gw.LoRaModulationInfo{
					Bandwidth:       125,
					SpreadingFactor: 7,
					CodeRate:        "3/4",
				},
			},
		},
	}

	zeroFreqKey, err := deduplicateOnTXInfoClearingFrequency(uplinkFrame)

	// Original frame freq remains unchanged
	assert.Equal(t, uplinkFrame.TxInfo.Frequency, originalFreq)

	assert.NoError(t, err)
	assert.NotNil(t, zeroFreqKey)

	txInfob, err := hex.DecodeString(zeroFreqKey)
	assert.NoError(t, err)

	var txInfo gw.UplinkTXInfo
	proto.Unmarshal(txInfob, &txInfo)
	assert.Equal(t, uint32(0), txInfo.Frequency)
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
	type Gateways struct {
		eui64         lorawan.EUI64
		frequencyUsed uint32
	}

	testTable := []struct {
		Name               string
		PHYPayload         lorawan.PHYPayload
		Gateways           []Gateways
		Count              int
		IgnoreFreqForDeDup bool
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
			[]Gateways{
				{
					eui64:         lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 1},
					frequencyUsed: 868100000,
				},
			},
			1,
			false,
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
			[]Gateways{
				{
					eui64:         lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 1},
					frequencyUsed: 868100000,
				},
				{
					eui64:         lorawan.EUI64{2, 2, 2, 2, 2, 2, 2, 2},
					frequencyUsed: 868100000,
				},
			},
			2,
			false,
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
			[]Gateways{
				{
					eui64:         lorawan.EUI64{3, 1, 1, 1, 1, 1, 1, 1},
					frequencyUsed: 868100000,
				},
				{
					eui64:         lorawan.EUI64{3, 2, 2, 2, 2, 2, 2, 2},
					frequencyUsed: 868100000,
				},
				{
					eui64:         lorawan.EUI64{3, 2, 2, 2, 2, 2, 2, 2},
					frequencyUsed: 868100000,
				},
			},
			2,
			false,
		},
		{
			"two items expected (two collected from same gateway, different frequencies, but ignoring that in dedup. Called once only.)",
			lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MIC:        [4]byte{3, 2, 3, 4},
				MACPayload: &lorawan.MACPayload{},
			},
			[]Gateways{
				{
					eui64:         lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 1},
					frequencyUsed: 868100000,
				},
				{
					eui64:         lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 1},
					frequencyUsed: 868300000,
				},
			},
			2,
			true,
		},
		// https://github.com/brocaar/chirpstack-network-server/issues/557#issuecomment-968719234
		//{
		//	"The unexpected/expected case. This will call *twice*, and fail",
		//	lorawan.PHYPayload{
		//		MHDR: lorawan.MHDR{
		//			MType: lorawan.UnconfirmedDataUp,
		//			Major: lorawan.LoRaWANR1,
		//		},
		//		MIC:        [4]byte{3, 2, 3, 4},
		//		MACPayload: &lorawan.MACPayload{},
		//	},
		//	[]Gateways{
		//		{
		//			eui64:         lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 1},
		//			frequencyUsed: 868100000,
		//		},
		//		{
		//			eui64:         lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 1},
		//			frequencyUsed: 868300000,
		//		},
		//	},
		//	1,
		//	false,
		//},
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
				g := tst.Gateways[i].eui64
				freq := tst.Gateways[i].frequencyUsed
				phyB, err := tst.PHYPayload.MarshalBinary()
				assert.NoError(err)

				wg.Add(1)
				packet := gw.UplinkFrame{
					RxInfo: &gw.UplinkRXInfo{
						GatewayId: g[:],
					},
					TxInfo: &gw.UplinkTXInfo{
						Frequency: freq,
					},
					PhyPayload: phyB,
				}
				assert.NoError(helpers.SetUplinkTXInfoDataRate(packet.TxInfo, 0, band.Band()))

				go func(packet gw.UplinkFrame) {
					assert.NoError(collectAndCallOnce(packet, tst.IgnoreFreqForDeDup, cb))
					wg.Done()
				}(packet)
			}
			wg.Wait()

			// collectAndCallOnce
			assert.Equal(1, called)
			assert.Equal(tst.Count, received)
		})
	}
}

func TestCollect(t *testing.T) {
	suite.Run(t, new(CollectTestSuite))
}
