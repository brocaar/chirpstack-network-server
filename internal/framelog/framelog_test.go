package framelog

import (
	"context"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	commonPB "github.com/brocaar/loraserver/api/common"
	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/test"
	"github.com/brocaar/lorawan"
)

type FrameLogTestSuite struct {
	suite.Suite
	test.DatabaseTestSuiteBase

	GatewayID lorawan.EUI64
	DevEUI    lorawan.EUI64
}

func (ts *FrameLogTestSuite) SetupSuite() {
	ts.DatabaseTestSuiteBase.SetupSuite()

	ts.GatewayID = lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}
	ts.DevEUI = lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1}
}

func (ts *FrameLogTestSuite) TestGetFrameLogForGateway() {
	assert := require.New(ts.T())

	logChannel := make(chan FrameLog, 1)
	ctx := context.Background()
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		err := GetFrameLogForGateway(cctx, ts.RedisPool(), ts.GatewayID, logChannel)
		assert.NoError(err)
	}()

	time.Sleep(100 * time.Millisecond)

	ts.T().Run("LogUplinkFrameForGateways", func(t *testing.T) {
		assert := require.New(t)

		uplinkFrameSet := gw.UplinkFrameSet{
			PhyPayload: []byte{1, 2, 3, 4},
			TxInfo: &gw.UplinkTXInfo{
				Frequency:  868100000,
				Modulation: commonPB.Modulation_LORA,
				ModulationInfo: &gw.UplinkTXInfo_LoraModulationInfo{
					LoraModulationInfo: &gw.LoRaModulationInfo{
						SpreadingFactor: 7,
					},
				},
			},
			RxInfo: []*gw.UplinkRXInfo{
				{
					GatewayId: ts.GatewayID[:],
					LoraSnr:   5.5,
				},
			},
		}
		assert.NoError(LogUplinkFrameForGateways(ts.RedisPool(), uplinkFrameSet))
		frameLog := <-logChannel
		assert.True(proto.Equal(&uplinkFrameSet, frameLog.UplinkFrame))
	})

	ts.T().Run("LogDownlinkFrameForGateway", func(t *testing.T) {
		assert := require.New(t)
		downlinkFrame := gw.DownlinkFrame{
			PhyPayload: []byte{1, 2, 3, 4},
			TxInfo: &gw.DownlinkTXInfo{
				GatewayId: ts.GatewayID[:],
			},
		}

		assert.NoError(LogDownlinkFrameForGateway(ts.RedisPool(), downlinkFrame))
		downlinkFrame.TxInfo.XXX_sizecache = 0

		assert.Equal(FrameLog{
			DownlinkFrame: &downlinkFrame,
		}, <-logChannel)
	})
}

func (ts *FrameLogTestSuite) TestGetFrameLogForDevice() {
	assert := require.New(ts.T())

	logChannel := make(chan FrameLog, 1)
	ctx := context.Background()
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		err := GetFrameLogForDevice(cctx, ts.RedisPool(), ts.DevEUI, logChannel)
		assert.NoError(err)
	}()

	time.Sleep(100 * time.Millisecond)

	ts.T().Run("LogUplinkFrameForDevEUI", func(t *testing.T) {
		assert := require.New(t)

		uplinkFrameSet := gw.UplinkFrameSet{
			PhyPayload: []byte{1, 2, 3, 4},
			TxInfo: &gw.UplinkTXInfo{
				Frequency:  868100000,
				Modulation: commonPB.Modulation_LORA,
				ModulationInfo: &gw.UplinkTXInfo_LoraModulationInfo{
					LoraModulationInfo: &gw.LoRaModulationInfo{
						SpreadingFactor: 7,
					},
				},
			},
			RxInfo: []*gw.UplinkRXInfo{
				{
					GatewayId: ts.GatewayID[:],
					LoraSnr:   5.5,
				},
			},
		}

		assert.NoError(LogUplinkFrameForDevEUI(ts.RedisPool(), ts.DevEUI, uplinkFrameSet))
		frameLog := <-logChannel
		assert.True(proto.Equal(frameLog.UplinkFrame, &uplinkFrameSet))
	})

	ts.T().Run("LogDownlinkFrameForDevEUI", func(t *testing.T) {
		assert := require.New(t)

		downlinkFrame := gw.DownlinkFrame{
			PhyPayload: []byte{1, 2, 3, 4},
			TxInfo: &gw.DownlinkTXInfo{
				GatewayId: ts.GatewayID[:],
			},
		}

		assert.NoError(LogDownlinkFrameForDevEUI(ts.RedisPool(), ts.DevEUI, downlinkFrame))
		downlinkFrame.TxInfo.XXX_sizecache = 0

		assert.Equal(FrameLog{
			DownlinkFrame: &downlinkFrame,
		}, <-logChannel)
	})
}

func TestFrameLog(t *testing.T) {
	suite.Run(t, new(FrameLogTestSuite))
}
