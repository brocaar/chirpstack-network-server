package framelog

import (
	"context"
	"fmt"
	"time"

	proto "github.com/golang/protobuf/proto"
	"github.com/gomodule/redigo/redis"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/config"
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
	UplinkFrame   *gw.UplinkFrameSet
	DownlinkFrame *gw.DownlinkFrame
}

// LogUplinkFrameForGateways logs the given frame to all the gateway pub-sub keys.
func LogUplinkFrameForGateways(uplinkFrameSet gw.UplinkFrameSet) error {
	c := config.C.Redis.Pool.Get()
	defer c.Close()

	c.Send("MULTI")
	for _, rx := range uplinkFrameSet.RxInfo {
		var id lorawan.EUI64
		copy(id[:], rx.GatewayId)

		frameLog := gw.UplinkFrameSet{
			PhyPayload: uplinkFrameSet.PhyPayload,
			TxInfo:     uplinkFrameSet.TxInfo,
			RxInfo:     []*gw.UplinkRXInfo{rx},
		}

		b, err := proto.Marshal(&frameLog)
		if err != nil {
			return errors.Wrap(err, "marshal uplink frame-set error")
		}

		key := fmt.Sprintf(gatewayFrameLogUplinkPubSubKeyTempl, id)
		c.Send("PUBLISH", key, b)
	}
	_, err := c.Do("EXEC")
	if err != nil {
		return errors.Wrap(err, "publish frame to gateway channel error")
	}

	return nil
}

// LogDownlinkFrameForGateway logs the given frame to the gateway pub-sub key.
func LogDownlinkFrameForGateway(frame gw.DownlinkFrame) error {
	var id lorawan.EUI64
	copy(id[:], frame.TxInfo.GatewayId)

	c := config.C.Redis.Pool.Get()
	defer c.Close()

	key := fmt.Sprintf(gatewayFrameLogDownlinkPubSubKeyTempl, id)

	b, err := proto.Marshal(&frame)
	if err != nil {
		return errors.Wrap(err, "marshal downlink frame error")
	}

	_, err = c.Do("PUBLISH", key, b)
	if err != nil {
		return errors.Wrap(err, "publish frame to gateway channel error")
	}
	return nil
}

// LogDownlinkFrameForDevEUI logs the given frame to the device pub-sub key.
func LogDownlinkFrameForDevEUI(devEUI lorawan.EUI64, frame gw.DownlinkFrame) error {
	c := config.C.Redis.Pool.Get()
	defer c.Close()

	key := fmt.Sprintf(deviceFrameLogDownlinkPubSubKeyTempl, devEUI)

	b, err := proto.Marshal(&frame)
	if err != nil {
		return errors.Wrap(err, "marshal downlink frame error")
	}

	_, err = c.Do("PUBLISH", key, b)
	if err != nil {
		return errors.Wrap(err, "publish frame to device channel error")
	}
	return nil
}

// LogUplinkFrameForDevEUI logs the given frame to the pub-sub key of the given DevEUI.
func LogUplinkFrameForDevEUI(devEUI lorawan.EUI64, frame gw.UplinkFrameSet) error {
	c := config.C.Redis.Pool.Get()
	defer c.Close()

	b, err := proto.Marshal(&frame)
	if err != nil {
		return errors.Wrap(err, "marshal uplink frame error")
	}

	key := fmt.Sprintf(deviceFrameLogUplinkPubSubKeyTempl, devEUI)
	_, err = c.Do("PUBLISH", key, b)
	if err != nil {
		return errors.Wrap(err, "publish frame to device channel error")
	}
	return nil
}

// GetFrameLogForGateway subscribes to the uplink and downlink frame logs
// for the given gateway and sends this to the given channel.
func GetFrameLogForGateway(ctx context.Context, gatewayID lorawan.EUI64, frameLogChan chan FrameLog) error {
	uplinkKey := fmt.Sprintf(gatewayFrameLogUplinkPubSubKeyTempl, gatewayID)
	downlinkKey := fmt.Sprintf(gatewayFrameLogDownlinkPubSubKeyTempl, gatewayID)
	return getFrameLogs(ctx, uplinkKey, downlinkKey, frameLogChan)
}

// GetFrameLogForDevice subscribes to the uplink and downlink frame logs
// for the given device and sends this to the given channel.
func GetFrameLogForDevice(ctx context.Context, devEUI lorawan.EUI64, frameLogChan chan FrameLog) error {
	uplinkKey := fmt.Sprintf(deviceFrameLogUplinkPubSubKeyTempl, devEUI)
	downlinkKey := fmt.Sprintf(deviceFrameLogDownlinkPubSubKeyTempl, devEUI)
	return getFrameLogs(ctx, uplinkKey, downlinkKey, frameLogChan)
}

func getFrameLogs(ctx context.Context, uplinkKey, downlinkKey string, frameLogChan chan FrameLog) error {
	c := config.C.Redis.Pool.Get()
	defer c.Close()

	psc := redis.PubSubConn{Conn: c}
	if err := psc.Subscribe(uplinkKey, downlinkKey); err != nil {
		return errors.Wrap(err, "subscribe error")
	}

	done := make(chan error, 1)

	go func() {
		for {
			switch v := psc.Receive().(type) {
			case redis.Message:
				fl, err := redisMessageToFrameLog(v, uplinkKey, downlinkKey)
				if err != nil {
					log.WithError(err).Error("decode message error")
				} else {
					frameLogChan <- fl
				}
			case redis.Subscription:
				if v.Count == 0 {
					done <- nil
					return
				}
			case error:
				done <- v
				return
			}
		}
	}()

	// todo: make this a config value?
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

loop:
	for {
		select {
		case <-ticker.C:
			if err := psc.Ping(""); err != nil {
				log.WithError(err).Error("subscription ping error")
				break loop
			}
		case <-ctx.Done():
			break loop
		case err := <-done:
			return err
		}
	}

	if err := psc.Unsubscribe(); err != nil {
		return errors.Wrap(err, "unsubscribe error")
	}

	return <-done
}

func redisMessageToFrameLog(msg redis.Message, uplinkKey, downlinkKey string) (FrameLog, error) {
	var fl FrameLog

	if msg.Channel == uplinkKey {
		fl.UplinkFrame = &gw.UplinkFrameSet{}
		if err := proto.Unmarshal(msg.Data, fl.UplinkFrame); err != nil {
			return fl, errors.Wrap(err, "unmarshal uplink frame-set error")
		}
	}

	if msg.Channel == downlinkKey {
		fl.DownlinkFrame = &gw.DownlinkFrame{}
		if err := proto.Unmarshal(msg.Data, fl.DownlinkFrame); err != nil {
			return fl, errors.Wrap(err, "unmarshal downlink frame error")
		}
	}

	return fl, nil
}
