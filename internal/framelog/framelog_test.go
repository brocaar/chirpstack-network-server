package framelog

import (
	"context"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/brocaar/chirpstack-api/go/v3/common"
	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/brocaar/chirpstack-api/go/v3/ns"
	"github.com/brocaar/chirpstack-network-server/v3/internal/config"
	"github.com/brocaar/chirpstack-network-server/v3/internal/storage"
	"github.com/brocaar/chirpstack-network-server/v3/internal/test"
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
	conf.Monitoring.PerDeviceFrameLogMaxHistory = 10
	conf.Monitoring.PerGatewayFrameLogMaxHistory = 10
	conf.Monitoring.DeviceFrameLogMaxHistory = 10
	conf.Monitoring.GatewayFrameLogMaxHistory = 10
	config.Set(conf)

	assert.NoError(storage.Setup(conf))

	ts.GatewayID = lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}
	ts.DevEUI = lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1}
}

func (ts *FrameLogTestSuite) SetupTest() {
	storage.RedisClient().FlushAll(context.Background())
}

func (ts *FrameLogTestSuite) TestLogUplinkFrameForGateways() {
	assert := require.New(ts.T())

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
		MType:   common.MType_UnconfirmedDataUp,
		DevAddr: []byte{4, 3, 2, 1},
		DevEui:  []byte{1, 2, 3, 4, 5, 6, 7, 8},
	}
	assert.NoError(LogUplinkFrameForGateways(context.Background(), uplinkFrameLog))

	val := storage.RedisClient().XRead(context.Background(), &redis.XReadArgs{
		Streams: []string{globalGatewayFrameStreamKey, "0"},
		Count:   1,
		Block:   0,
	}).Val()

	assert.Len(val, 1)
	assert.Len(val[0].Messages, 1)

	var pl ns.UplinkFrameLog
	assert.NoError(proto.Unmarshal([]byte(val[0].Messages[0].Values["up"].(string)), &pl))
	assert.NotNil(pl.PublishedAt)
	pl.PublishedAt = nil
	assert.True(proto.Equal(&uplinkFrameLog, &pl))
}

func (ts *FrameLogTestSuite) TestLogDownlinkFrameForGateway() {
	assert := require.New(ts.T())

	downlinkFrameLog := ns.DownlinkFrameLog{
		GatewayId:  ts.GatewayID[:],
		PhyPayload: []byte{1, 2, 3, 4},
		TxInfo:     &gw.DownlinkTXInfo{},
	}

	assert.NoError(LogDownlinkFrameForGateway(context.Background(), downlinkFrameLog))

	val := storage.RedisClient().XRead(context.Background(), &redis.XReadArgs{
		Streams: []string{globalGatewayFrameStreamKey, "0"},
		Count:   1,
		Block:   0,
	}).Val()

	assert.Len(val, 1)
	assert.Len(val[0].Messages, 1)

	var pl ns.DownlinkFrameLog
	assert.NoError(proto.Unmarshal([]byte(val[0].Messages[0].Values["down"].(string)), &pl))
	assert.NotNil(pl.PublishedAt)
	pl.PublishedAt = nil
	assert.True(proto.Equal(&downlinkFrameLog, &pl))
}

func (ts *FrameLogTestSuite) TestLogUplinkFrameForDevEUI() {
	assert := require.New(ts.T())

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

	assert.NoError(LogUplinkFrameForDevEUI(context.Background(), ts.DevEUI, uplinkFrameLog))

	val := storage.RedisClient().XRead(context.Background(), &redis.XReadArgs{
		Streams: []string{globalDeviceFrameStreamKey, "0"},
		Count:   1,
		Block:   0,
	}).Val()

	assert.Len(val, 1)
	assert.Len(val[0].Messages, 1)

	var pl ns.UplinkFrameLog
	assert.NoError(proto.Unmarshal([]byte(val[0].Messages[0].Values["up"].(string)), &pl))
	assert.NotNil(pl.PublishedAt)
	pl.PublishedAt = nil
	assert.True(proto.Equal(&uplinkFrameLog, &pl))
}

func (ts *FrameLogTestSuite) TestLogDownlinkFrameForDevEUI() {
	assert := require.New(ts.T())

	downlinkFrameLog := ns.DownlinkFrameLog{
		PhyPayload: []byte{1, 2, 3, 4},
		GatewayId:  ts.GatewayID[:],
		TxInfo:     &gw.DownlinkTXInfo{},
	}

	assert.NoError(LogDownlinkFrameForDevEUI(context.Background(), ts.DevEUI, downlinkFrameLog))

	val := storage.RedisClient().XRead(context.Background(), &redis.XReadArgs{
		Streams: []string{globalDeviceFrameStreamKey, "0"},
		Count:   1,
		Block:   0,
	}).Val()

	assert.Len(val, 1)
	assert.Len(val[0].Messages, 1)

	var pl ns.DownlinkFrameLog
	assert.NoError(proto.Unmarshal([]byte(val[0].Messages[0].Values["down"].(string)), &pl))
	assert.NotNil(pl.PublishedAt)
	pl.PublishedAt = nil
	assert.True(proto.Equal(&downlinkFrameLog, &pl))

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
		assert.NotNil(frameLog.UplinkFrame.PublishedAt)
		frameLog.UplinkFrame.PublishedAt = nil
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

		pl := <-logChannel
		assert.NotNil(pl.DownlinkFrame.PublishedAt)
		pl.DownlinkFrame.PublishedAt = nil

		assert.Equal(FrameLog{
			DownlinkFrame: &downlinkFrameLog,
		}, pl)
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
		assert.NotNil(frameLog.UplinkFrame.PublishedAt)
		frameLog.UplinkFrame.PublishedAt = nil
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

		frameLog := <-logChannel
		assert.NotNil(frameLog.DownlinkFrame.PublishedAt)
		frameLog.DownlinkFrame.PublishedAt = nil

		assert.Equal(FrameLog{
			DownlinkFrame: &downlinkFrameLog,
		}, frameLog)
	})
}

func TestFrameLog(t *testing.T) {
	suite.Run(t, new(FrameLogTestSuite))
}
