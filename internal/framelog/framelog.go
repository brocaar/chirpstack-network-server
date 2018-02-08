package framelog

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"

	"github.com/garyburd/redigo/redis"

	"github.com/brocaar/lorawan"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/models"
	"github.com/pkg/errors"
)

const (
	gatewayFrameLogUplinkPubSubKeyTempl   = "lora:ns:gw:%s:pubsub:frame:uplink"
	gatewayFrameLogDownlinkPubSubKeyTempl = "lora:ns:gw:%s:pubsub:frame:downlink"
	deviceFrameLogUplinkPubSubKeyTempl    = "lora:ns:device:%d:pubsub:frame:uplink"
	deviceFrameLogDownlinkPubSubKeyTempl  = "lora:ns:device:%d:pubsub:frame:downlink"
)

// UplinkFrameLog contains the details of an uplink frame.
type UplinkFrameLog struct {
	PHYPayload lorawan.PHYPayload
	TXInfo     models.TXInfo
	RXInfoSet  []models.RXInfo
}

// DownlinkFrameLog contains the details of a downlink frame.
type DownlinkFrameLog struct {
	PHYPayload lorawan.PHYPayload
	TXInfo     gw.TXInfo
}

// FrameLog contains either an uplink or downlink frame.
type FrameLog struct {
	UplinkFrame   *UplinkFrameLog
	DownlinkFrame *DownlinkFrameLog
}

// LogUplinkFrameForGateways logs the given frame to all the gateway pub-sub keys.
func LogUplinkFrameForGateways(rxPacket models.RXPacket) error {
	c := common.RedisPool.Get()
	defer c.Close()

	c.Send("MULTI")
	for _, rx := range rxPacket.RXInfoSet {
		frameLog := UplinkFrameLog{
			PHYPayload: rxPacket.PHYPayload,
			TXInfo:     rxPacket.TXInfo,
			RXInfoSet:  []models.RXInfo{rx},
		}

		key := fmt.Sprintf(gatewayFrameLogUplinkPubSubKeyTempl, rx.MAC)
		var buf bytes.Buffer
		if err := gob.NewEncoder(&buf).Encode(frameLog); err != nil {
			return errors.Wrap(err, "gob encode error")
		}
		c.Send("PUBLISH", key, buf.Bytes())
	}
	_, err := c.Do("EXEC")
	if err != nil {
		return errors.Wrap(err, "publish frame to gateway channel error")
	}

	return nil
}

// LogDownlinkFrameForGateway logs the given frame to the gateway pub-sub key.
func LogDownlinkFrameForGateway(frame DownlinkFrameLog) error {
	c := common.RedisPool.Get()
	defer c.Close()

	key := fmt.Sprintf(gatewayFrameLogDownlinkPubSubKeyTempl, frame.TXInfo.MAC)
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(frame); err != nil {
		return errors.Wrap(err, "gob encode error")
	}

	_, err := c.Do("PUBLISH", key, buf.Bytes())
	if err != nil {
		return errors.Wrap(err, "publish frame to gateway channel error")
	}
	return nil
}

// LogDownlinkFrameForDevEUI logs the given frame to the device pub-sub key.
func LogDownlinkFrameForDevEUI(devEUI lorawan.EUI64, frame DownlinkFrameLog) error {
	c := common.RedisPool.Get()
	defer c.Close()

	key := fmt.Sprintf(deviceFrameLogDownlinkPubSubKeyTempl, devEUI)
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(frame); err != nil {
		return errors.Wrap(err, "gob encode error")
	}

	_, err := c.Do("PUBLISH", key, buf.Bytes())
	if err != nil {
		return errors.Wrap(err, "publish frame to device channel error")
	}
	return nil
}

// LogUplinkFrameForDevEUI logs the given frame to the pub-sub key of the given DevEUI.
func LogUplinkFrameForDevEUI(devEUI lorawan.EUI64, rxPacket models.RXPacket) error {
	c := common.RedisPool.Get()
	defer c.Close()

	frameLog := UplinkFrameLog{
		PHYPayload: rxPacket.PHYPayload,
		TXInfo:     rxPacket.TXInfo,
		RXInfoSet:  rxPacket.RXInfoSet,
	}
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(frameLog); err != nil {
		return errors.Wrap(err, "gob encode error")
	}
	key := fmt.Sprintf(deviceFrameLogUplinkPubSubKeyTempl, devEUI)

	_, err := c.Do("PUBLISH", key, buf.Bytes())
	if err != nil {
		return errors.Wrap(err, "publish frame to device channel error")
	}
	return nil
}

// GetFrameLogForGateway subscribes to the uplink and downlink frame logs
// for the given gateway and sends this to the given channel.
func GetFrameLogForGateway(ctx context.Context, mac lorawan.EUI64, frameLogChan chan FrameLog) error {
	uplinkKey := fmt.Sprintf(gatewayFrameLogUplinkPubSubKeyTempl, mac)
	downlinkKey := fmt.Sprintf(gatewayFrameLogDownlinkPubSubKeyTempl, mac)
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
	c := common.RedisPool.Get()
	defer c.Close()
	var done bool

	go func() {
		done = true
		<-ctx.Done()
		c.Close()
	}()

	psc := redis.PubSubConn{Conn: c}
	if err := psc.Subscribe(uplinkKey, downlinkKey); err != nil {
		return errors.Wrap(err, "subscribe error")
	}
	for {
		switch v := psc.Receive().(type) {
		case redis.Message:
			fl, err := redisMessageToFrameLog(v, uplinkKey, downlinkKey)
			if err != nil {
				return errors.Wrap(err, "decode message error")
			}
			frameLogChan <- fl

		case error:
			if done {
				return nil
			}
			return errors.Wrap(v, "receive error")
		}
	}
}

func redisMessageToFrameLog(msg redis.Message, uplinkKey, downlinkKey string) (FrameLog, error) {
	var fl FrameLog

	if msg.Channel == uplinkKey {
		fl.UplinkFrame = &UplinkFrameLog{}
		if err := gob.NewDecoder(bytes.NewReader(msg.Data)).Decode(fl.UplinkFrame); err != nil {
			return fl, errors.Wrap(err, "gob decode uplink frame error")
		}
	}

	if msg.Channel == downlinkKey {
		fl.DownlinkFrame = &DownlinkFrameLog{}
		if err := gob.NewDecoder(bytes.NewReader(msg.Data)).Decode(fl.DownlinkFrame); err != nil {
			return fl, errors.Wrap(err, "gob decode downlink frame error")
		}
	}

	return fl, nil
}
