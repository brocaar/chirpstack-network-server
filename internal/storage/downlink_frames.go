package storage

import (
	"context"
	"fmt"
	"time"

	proto "github.com/golang/protobuf/proto"
	"github.com/gomodule/redigo/redis"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/logging"
	"github.com/brocaar/lorawan"
)

const downlinkFramesTTL = time.Second * 10
const downlinkFramesKeyTempl = "lora:ns:frames:%d"
const downlinkFramesDevEUIKeyTempl = "lora:ns:frames:deveui:%d"

// SaveDownlinkFrames saves the given downlink-frames. The downlink-frames
// must share the same token!
func SaveDownlinkFrames(ctx context.Context, p *redis.Pool, devEUI lorawan.EUI64, frames []gw.DownlinkFrame) error {
	if len(frames) == 0 {
		return nil
	}

	var token uint32
	var frameBytes [][]byte
	for _, frame := range frames {
		token = frame.Token
		b, err := proto.Marshal(&frame)
		if err != nil {
			return errors.Wrap(err, "marshal proto error")
		}
		frameBytes = append(frameBytes, b)
	}

	c := p.Get()
	defer c.Close()

	exp := int64(downlinkFramesTTL) / int64(time.Millisecond)
	c.Send("MULTI")

	// store frames
	key := fmt.Sprintf(downlinkFramesKeyTempl, token)
	for i := range frameBytes {
		c.Send("RPUSH", key, frameBytes[i])
	}
	c.Send("PEXPIRE", key, exp)

	// store pointer to deveui
	key = fmt.Sprintf(downlinkFramesDevEUIKeyTempl, token)
	c.Send("PSETEX", key, exp, devEUI[:])

	// execute
	if _, err := c.Do("EXEC"); err != nil {
		return errors.Wrap(err, "exec error")
	}

	log.WithFields(log.Fields{
		"token":   token,
		"dev_eui": devEUI,
		"ctx_id":  ctx.Value(logging.ContextIDKey),
	}).Info("downlink-frames saved")

	return nil
}

// PopDownlinkFrame returns the first downlink-frame for the given token.
func PopDownlinkFrame(ctx context.Context, p *redis.Pool, token uint32) (lorawan.EUI64, gw.DownlinkFrame, error) {
	var out gw.DownlinkFrame
	var devEUI lorawan.EUI64

	c := p.Get()
	defer c.Close()

	b, err := redis.Bytes(c.Do("LPOP", fmt.Sprintf(downlinkFramesKeyTempl, token)))
	if err != nil {
		if err == redis.ErrNil {
			return lorawan.EUI64{}, gw.DownlinkFrame{}, ErrDoesNotExist
		}
		return lorawan.EUI64{}, gw.DownlinkFrame{}, errors.Wrap(err, "lpop error")
	}
	err = proto.Unmarshal(b, &out)
	if err != nil {
		return lorawan.EUI64{}, gw.DownlinkFrame{}, errors.Wrap(err, "proto unmarshal error")
	}

	b, err = redis.Bytes(c.Do("GET", fmt.Sprintf(downlinkFramesDevEUIKeyTempl, token)))
	if err != nil {
		if err == redis.ErrNil {
			return lorawan.EUI64{}, gw.DownlinkFrame{}, ErrDoesNotExist
		}
		return lorawan.EUI64{}, gw.DownlinkFrame{}, errors.Wrap(err, "get error")
	}

	copy(devEUI[:], b)

	return devEUI, out, nil
}
