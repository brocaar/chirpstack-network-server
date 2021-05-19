package framelog

import (
	"context"

	"github.com/go-redis/redis/v8"
	proto "github.com/golang/protobuf/proto"
	"github.com/pkg/errors"

	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/brocaar/chirpstack-api/go/v3/ns"
	"github.com/brocaar/chirpstack-network-server/v3/internal/config"
	"github.com/brocaar/chirpstack-network-server/v3/internal/storage"
	"github.com/brocaar/lorawan"
)

const (
	globalGatewayFrameStreamKey = "lora:ns:gw:stream:frame"
	globalDeviceFrameStreamKey  = "lora:ns:device:stream:frame"
	gatewayFrameLogStreamKey    = "lora:ns:gw:%s:stream:frame"
	deviceFrameLogStreamKey     = "lora:ns:device:%s:stream:frame"
)

// FrameLog contains either an uplink or downlink frame.
type FrameLog struct {
	UplinkFrame   *ns.UplinkFrameLog
	DownlinkFrame *ns.DownlinkFrameLog
}

// LogUplinkFrameForGateways logs the given frame to all the gateway pub-sub keys.
func LogUplinkFrameForGateways(ctx context.Context, frame ns.UplinkFrameLog) error {
	conf := config.Get()

	for _, rx := range frame.RxInfo {
		var id lorawan.EUI64
		copy(id[:], rx.GatewayId)

		frameLog := ns.UplinkFrameLog{
			PhyPayload: frame.PhyPayload,
			TxInfo:     frame.TxInfo,
			RxInfo:     []*gw.UplinkRXInfo{rx},
			MType:      frame.MType,
			DevAddr:    frame.DevAddr,
			DevEui:     frame.DevEui,
		}

		b, err := proto.Marshal(&frameLog)
		if err != nil {
			return errors.Wrap(err, "marshal uplink frame-set error")
		}

		// per gateway stream
		if conf.Monitoring.PerGatewayFrameLogMaxHistory > 0 {
			key := storage.GetRedisKey(gatewayFrameLogStreamKey, id)
			pipe := storage.RedisClient().TxPipeline()

			pipe.XAdd(ctx, &redis.XAddArgs{
				Stream:       key,
				MaxLenApprox: conf.Monitoring.PerGatewayFrameLogMaxHistory,
				Values: map[string]interface{}{
					"up": b,
				},
			})
			pipe.Expire(ctx, key, conf.NetworkServer.DeviceSessionTTL)

			_, err := pipe.Exec(ctx)
			if err != nil {
				return errors.Wrap(err, "redis xadd error")
			}
		}

		// global gateway stream
		if conf.Monitoring.GatewayFrameLogMaxHistory > 0 {
			key := storage.GetRedisKey(globalGatewayFrameStreamKey)
			if err := storage.RedisClient().XAdd(ctx, &redis.XAddArgs{
				Stream:       key,
				MaxLenApprox: conf.Monitoring.GatewayFrameLogMaxHistory,
				Values: map[string]interface{}{
					"up": b,
				},
			}).Err(); err != nil {
				return errors.Wrap(err, "redis xadd error")
			}
		}
	}

	return nil
}

// LogDownlinkFrameForGateway logs the given frame to the gateway pub-sub key.
func LogDownlinkFrameForGateway(ctx context.Context, frame ns.DownlinkFrameLog) error {
	conf := config.Get()
	var id lorawan.EUI64
	copy(id[:], frame.GatewayId)

	b, err := proto.Marshal(&frame)
	if err != nil {
		return errors.Wrap(err, "marshal downlink frame error")
	}

	// per gateway stream
	if conf.Monitoring.PerGatewayFrameLogMaxHistory > 0 {
		key := storage.GetRedisKey(gatewayFrameLogStreamKey, id)
		pipe := storage.RedisClient().TxPipeline()

		pipe.XAdd(ctx, &redis.XAddArgs{
			Stream: key,
			MaxLen: conf.Monitoring.PerGatewayFrameLogMaxHistory,
			Values: map[string]interface{}{
				"down": b,
			},
		})
		pipe.Expire(ctx, key, conf.NetworkServer.DeviceSessionTTL)

		_, err := pipe.Exec(ctx)
		if err != nil {
			return errors.Wrap(err, "redis xadd error")
		}
	}

	// global gateway stream
	if conf.Monitoring.GatewayFrameLogMaxHistory > 0 {
		key := storage.GetRedisKey(globalGatewayFrameStreamKey)
		if err := storage.RedisClient().XAdd(ctx, &redis.XAddArgs{
			Stream:       key,
			MaxLenApprox: conf.Monitoring.GatewayFrameLogMaxHistory,
			Values: map[string]interface{}{
				"down": b,
			},
		}).Err(); err != nil {
			return errors.Wrap(err, "redis xadd error")
		}
	}

	return nil
}

// LogUplinkFrameForDevEUI logs the given frame to the pub-sub key of the given DevEUI.
func LogUplinkFrameForDevEUI(ctx context.Context, devEUI lorawan.EUI64, frame ns.UplinkFrameLog) error {
	conf := config.Get()

	b, err := proto.Marshal(&frame)
	if err != nil {
		return errors.Wrap(err, "marshal uplink frame error")
	}

	// per device stream
	if conf.Monitoring.PerDeviceFrameLogMaxHistory > 0 {
		key := storage.GetRedisKey(deviceFrameLogStreamKey, devEUI)
		pipe := storage.RedisClient().TxPipeline()

		pipe.XAdd(ctx, &redis.XAddArgs{
			Stream: key,
			MaxLen: conf.Monitoring.PerDeviceFrameLogMaxHistory,
			Values: map[string]interface{}{
				"up": b,
			},
		})
		pipe.Expire(ctx, key, conf.NetworkServer.DeviceSessionTTL)

		_, err := pipe.Exec(ctx)
		if err != nil {
			return errors.Wrap(err, "redis xadd error")
		}
	}

	// global device stream
	if conf.Monitoring.DeviceFrameLogMaxHistory > 0 {
		key := storage.GetRedisKey(globalDeviceFrameStreamKey)
		if err := storage.RedisClient().XAdd(ctx, &redis.XAddArgs{
			Stream:       key,
			MaxLenApprox: conf.Monitoring.DeviceFrameLogMaxHistory,
			Values: map[string]interface{}{
				"up": b,
			},
		}).Err(); err != nil {
			return errors.Wrap(err, "redis xadd error")
		}
	}

	return nil
}

// LogDownlinkFrameForDevEUI logs the given frame to the device pub-sub key.
func LogDownlinkFrameForDevEUI(ctx context.Context, devEUI lorawan.EUI64, frame ns.DownlinkFrameLog) error {
	conf := config.Get()

	// per device
	b, err := proto.Marshal(&frame)
	if err != nil {
		return errors.Wrap(err, "marshal downlink frame error")
	}

	// per device stream
	if conf.Monitoring.PerDeviceFrameLogMaxHistory > 0 {
		key := storage.GetRedisKey(deviceFrameLogStreamKey, devEUI)
		pipe := storage.RedisClient().TxPipeline()

		pipe.XAdd(ctx, &redis.XAddArgs{
			Stream:       key,
			MaxLenApprox: conf.Monitoring.PerDeviceFrameLogMaxHistory,
			Values: map[string]interface{}{
				"down": b,
			},
		})
		pipe.Expire(ctx, key, conf.NetworkServer.DeviceSessionTTL)

		_, err := pipe.Exec(ctx)
		if err != nil {
			return errors.Wrap(err, "redis xadd error")
		}
	}

	// global device stream
	if conf.Monitoring.DeviceFrameLogMaxHistory > 0 {
		key := storage.GetRedisKey(globalDeviceFrameStreamKey)
		if err := storage.RedisClient().XAdd(ctx, &redis.XAddArgs{
			Stream:       key,
			MaxLenApprox: conf.Monitoring.DeviceFrameLogMaxHistory,
			Values: map[string]interface{}{
				"down": b,
			},
		}).Err(); err != nil {
			return errors.Wrap(err, "redis xadd error")
		}
	}

	return nil
}

// GetFrameLogForGateway subscribes to the uplink / downlink frame logs
// stream for the given GatewayID.
func GetFrameLogForGateway(ctx context.Context, gatewayID lorawan.EUI64, frameLogChan chan FrameLog) error {
	key := storage.GetRedisKey(gatewayFrameLogStreamKey, gatewayID)
	return getFrameLogs(ctx, key, 10, frameLogChan)
}

// GetFrameLogForDevice subscribes to the uplink / downlink frame logs
// stream for the given DevEUI.
func GetFrameLogForDevice(ctx context.Context, devEUI lorawan.EUI64, frameLogChan chan FrameLog) error {
	key := storage.GetRedisKey(deviceFrameLogStreamKey, devEUI)
	return getFrameLogs(ctx, key, 10, frameLogChan)
}

func getFrameLogs(ctx context.Context, key string, count int64, frameLogChan chan FrameLog) error {
	lastID := "0"

	for {
		resp, err := storage.RedisClient().XRead(ctx, &redis.XReadArgs{
			Streams: []string{key, lastID},
			Count:   count,
			Block:   0,
		}).Result()
		if err != nil {
			if err == context.Canceled {
				return nil
			}
			return errors.Wrap(err, "redis stream error")
		}

		if len(resp) != 1 {
			return errors.New("exactly one stream response expected")
		}

		for _, msg := range resp[0].Messages {
			lastID = msg.ID

			if val, ok := msg.Values["up"]; ok {
				b, ok := val.(string)
				if !ok {
					continue
				}

				fl := FrameLog{UplinkFrame: &ns.UplinkFrameLog{}}
				if err := proto.Unmarshal([]byte(b), fl.UplinkFrame); err != nil {
					return errors.Wrap(err, "unmarshal uplink frame error")
				}

				frameLogChan <- fl
			}

			if val, ok := msg.Values["down"]; ok {
				b, ok := val.(string)
				if !ok {
					continue
				}

				fl := FrameLog{DownlinkFrame: &ns.DownlinkFrameLog{}}
				if err := proto.Unmarshal([]byte(b), fl.DownlinkFrame); err != nil {
					return errors.Wrap(err, "unmarshal downlink frame error")
				}

				frameLogChan <- fl
			}
		}
	}
}
