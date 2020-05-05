package framelog

import (
	"context"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/brocaar/chirpstack-api/go/v3/common"
	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/brocaar/chirpstack-api/go/v3/ns"
	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/brocaar/chirpstack-network-server/internal/test"
	"github.com/brocaar/lorawan"
)

type FrameLogTestSuite struct {
	suite.Suite

	GatewayID lorawan.EUI64
	DevEUI    lorawan.EUI64
}

func (ts *FrameLogTestSuite) SetupSuite() {
	assert := require.New(ts.T())
	conf := test.GetConfig()
	assert.NoError(storage.Setup(conf))

	ts.GatewayID = lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}
	ts.DevEUI = lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1}
}

func (ts *FrameLogTestSuite) SetupTest() {
	storage.RedisClient().FlushAll()
}

func (ts *FrameLogTestSuite) TestGetFrameLogForGateway() {
	assert := require.New(ts.T())

	logChannel := make(chan FrameLog, 1)
	ctx := context.Background()
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		err := GetFrameLogForGateway(cctx, ts.GatewayID, logChannel)
		assert.NoError(err)
	}()

	time.Sleep(100 * time.Millisecond)

	ts.T().Run("LogUplinkFrameForGateways", func(t *testing.T) {
		assert := require.New(t)

		uplinkFrameLog := ns.UplinkFrameLog{
			PhyPayload: []byte{1, 2, 3, 4},
			TxInfo: &gw.UplinkTXInfo{
				Frequency:  868100000,
				Modulation: common.Modulation_LORA,
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
		assert.NoError(LogUplinkFrameForGateways(ctx, uplinkFrameLog))
		frameLog := <-logChannel
		assert.True(proto.Equal(&uplinkFrameLog, frameLog.UplinkFrame))
	})

	ts.T().Run("LogDownlinkFrameForGateway", func(t *testing.T) {
		assert := require.New(t)
		downlinkFrameLog := ns.DownlinkFrameLog{
			GatewayId:  ts.GatewayID[:],
			PhyPayload: []byte{1, 2, 3, 4},
			TxInfo:     &gw.DownlinkTXInfo{},
		}

		assert.NoError(LogDownlinkFrameForGateway(ctx, downlinkFrameLog))
		downlinkFrameLog.TxInfo.XXX_sizecache = 0

		assert.Equal(FrameLog{
			DownlinkFrame: &downlinkFrameLog,
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
		err := GetFrameLogForDevice(cctx, ts.DevEUI, logChannel)
		assert.NoError(err)
	}()

	time.Sleep(100 * time.Millisecond)

	ts.T().Run("LogUplinkFrameForDevEUI", func(t *testing.T) {
		assert := require.New(t)

		uplinkFrameLog := ns.UplinkFrameLog{
			PhyPayload: []byte{1, 2, 3, 4},
			TxInfo: &gw.UplinkTXInfo{
				Frequency:  868100000,
				Modulation: common.Modulation_LORA,
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

		assert.NoError(LogUplinkFrameForDevEUI(ctx, ts.DevEUI, uplinkFrameLog))
		frameLog := <-logChannel
		assert.True(proto.Equal(frameLog.UplinkFrame, &uplinkFrameLog))
	})

	ts.T().Run("LogDownlinkFrameForDevEUI", func(t *testing.T) {
		assert := require.New(t)

		downlinkFrameLog := ns.DownlinkFrameLog{
			PhyPayload: []byte{1, 2, 3, 4},
			GatewayId:  ts.GatewayID[:],
			TxInfo:     &gw.DownlinkTXInfo{},
		}

		assert.NoError(LogDownlinkFrameForDevEUI(ctx, ts.DevEUI, downlinkFrameLog))
		downlinkFrameLog.TxInfo.XXX_sizecache = 0

		assert.Equal(FrameLog{
			DownlinkFrame: &downlinkFrameLog,
		}, <-logChannel)
	})
}

func TestFrameLog(t *testing.T) {
	suite.Run(t, new(FrameLogTestSuite))
}
