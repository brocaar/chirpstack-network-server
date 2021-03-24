package framelog

import (
	"context"

	"github.com/go-redis/redis/v7"
	proto "github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/brocaar/chirpstack-api/go/v3/ns"
	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/brocaar/lorawan"
)

const (
	gatewayFrameLogUplinkPubSubKeyTempl   = "lora:ns:gw:%s:pubsub:frame:uplink"
	gatewayFrameLogDownlinkPubSubKeyTempl = "lora:ns:gw:%s:pubsub:frame:downlink"
	deviceFrameLogUplinkPubSubKeyTempl    = "lora:ns:device:%s:pubsub:frame:uplink"
	deviceFrameLogDownlinkPubSubKeyTempl  = "lora:ns:device:%s:pubsub:frame:downlink"
)

// FrameLog contains either an uplink or downlink frame.
type FrameLog struct {
	UplinkFrame   *ns.UplinkFrameLog
	DownlinkFrame *ns.DownlinkFrameLog
}

// LogUplinkFrameForGateways logs the given frame to all the gateway pub-sub keys.
func LogUplinkFrameForGateways(ctx context.Context, frame ns.UplinkFrameLog) error {
	pipe := storage.RedisClient().Pipeline()

	for _, rx := range frame.RxInfo {
		var id lorawan.EUI64
		copy(id[:], rx.GatewayId)

		frameLog := gw.UplinkFrameSet{
			PhyPayload: frame.PhyPayload,
			TxInfo:     frame.TxInfo,
			RxInfo:     []*gw.UplinkRXInfo{rx},
		}

		b, err := proto.Marshal(&frameLog)
		if err != nil {
			return errors.Wrap(err, "marshal uplink frame-set error")
		}

		key := storage.GetRedisKey(gatewayFrameLogUplinkPubSubKeyTempl, id)
		pipe.Publish(key, b)
	}

	_, err := pipe.Exec()
	if err != nil {
		return errors.Wrap(err, "publish frame to gateway channel error")
	}

	return nil
}

// LogDownlinkFrameForGateway logs the given frame to the gateway pub-sub key.
func LogDownlinkFrameForGateway(ctx context.Context, frame ns.DownlinkFrameLog) error {
	var id lorawan.EUI64
	copy(id[:], frame.GatewayId)

	key := storage.GetRedisKey(gatewayFrameLogDownlinkPubSubKeyTempl, id)

	b, err := proto.Marshal(&frame)
	if err != nil {
		return errors.Wrap(err, "marshal downlink frame error")
	}

	err = storage.RedisClient().Publish(key, b).Err()
	if err != nil {
		return errors.Wrap(err, "publish frame to gateway channel error")
	}
	return nil
}

// LogDownlinkFrameForDevEUI logs the given frame to the device pub-sub key.
func LogDownlinkFrameForDevEUI(ctx context.Context, devEUI lorawan.EUI64, frame ns.DownlinkFrameLog) error {
	key := storage.GetRedisKey(deviceFrameLogDownlinkPubSubKeyTempl, devEUI)

	b, err := proto.Marshal(&frame)
	if err != nil {
		return errors.Wrap(err, "marshal downlink frame error")
	}

	err = storage.RedisClient().Publish(key, b).Err()
	if err != nil {
		return errors.Wrap(err, "publish frame to device channel error")
	}

	return nil
}

// LogUplinkFrameForDevEUI logs the given frame to the pub-sub key of the given DevEUI.
func LogUplinkFrameForDevEUI(ctx context.Context, devEUI lorawan.EUI64, frame ns.UplinkFrameLog) error {
	b, err := proto.Marshal(&frame)
	if err != nil {
		return errors.Wrap(err, "marshal uplink frame error")
	}

	key := storage.GetRedisKey(deviceFrameLogUplinkPubSubKeyTempl, devEUI)

	err = storage.RedisClient().Publish(key, b).Err()
	if err != nil {
		return errors.Wrap(err, "publish frame to device channel error")
	}
	return nil
}

// GetFrameLogForGateway subscribes to the uplink and downlink frame logs
// for the given gateway and sends this to the given channel.
func GetFrameLogForGateway(ctx context.Context, gatewayID lorawan.EUI64, frameLogChan chan FrameLog) error {
	uplinkKey := storage.GetRedisKey(gatewayFrameLogUplinkPubSubKeyTempl, gatewayID)
	downlinkKey := storage.GetRedisKey(gatewayFrameLogDownlinkPubSubKeyTempl, gatewayID)
	return getFrameLogs(ctx, uplinkKey, downlinkKey, frameLogChan)
}

// GetFrameLogForDevice subscribes to the uplink and downlink frame logs
// for the given device and sends this to the given channel.
func GetFrameLogForDevice(ctx context.Context, devEUI lorawan.EUI64, frameLogChan chan FrameLog) error {
	uplinkKey := storage.GetRedisKey(deviceFrameLogUplinkPubSubKeyTempl, devEUI)
	downlinkKey := storage.GetRedisKey(deviceFrameLogDownlinkPubSubKeyTempl, devEUI)
	return getFrameLogs(ctx, uplinkKey, downlinkKey, frameLogChan)
}

func getFrameLogs(ctx context.Context, uplinkKey, downlinkKey string, frameLogChan chan FrameLog) error {
	sub := storage.RedisClient().Subscribe(uplinkKey, downlinkKey)
	_, err := sub.Receive()
	if err != nil {
		return errors.Wrap(err, "subscribe error")
	}

	ch := sub.Channel()

	for {
		select {
		case msg := <-ch:
			if msg == nil {
				continue
			}

			fl, err := redisMessageToFrameLog(msg, uplinkKey, downlinkKey)
			if err != nil {
				log.WithError(err).Error("decode message error")
			} else {
				frameLogChan <- fl
			}
		case <-ctx.Done():
			// This will also close the channel
			sub.Close()
			return nil
		}
	}
}

func redisMessageToFrameLog(msg *redis.Message, uplinkKey, downlinkKey string) (FrameLog, error) {
	var fl FrameLog

	if msg.Channel == uplinkKey {
		fl.UplinkFrame = &ns.UplinkFrameLog{}
		if err := proto.Unmarshal([]byte(msg.Payload), fl.UplinkFrame); err != nil {
			return fl, errors.Wrap(err, "unmarshal uplink frame-set error")
		}
	}

	if msg.Channel == downlinkKey {
		fl.DownlinkFrame = &ns.DownlinkFrameLog{}
		if err := proto.Unmarshal([]byte(msg.Payload), fl.DownlinkFrame); err != nil {
			return fl, errors.Wrap(err, "unmarshal downlink frame error")
		}
	}

	return fl, nil
}
