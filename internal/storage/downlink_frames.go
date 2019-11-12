//go:generate protoc -I=. -I=../.. --go_out=. downlink_frames.proto

package storage

import (
	"context"
	"fmt"
	"time"

	proto "github.com/golang/protobuf/proto"
	"github.com/gomodule/redigo/redis"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/chirpstack-network-server/internal/logging"
)

const downlinkFramesTTL = time.Second * 10
const downlinkFramesKeyTempl = "lora:ns:frames:%d"

// SaveDownlinkFrames saves the given downlink-frames.
func SaveDownlinkFrames(ctx context.Context, p *redis.Pool, frames DownlinkFrames) error {
	c := p.Get()
	defer c.Close()

	b, err := proto.Marshal(&frames)
	if err != nil {
		return errors.Wrap(err, "marshal proto error")
	}

	key := fmt.Sprintf(downlinkFramesKeyTempl, frames.Token)
	exp := int64(downlinkFramesTTL) / int64(time.Millisecond)

	_, err = c.Do("PSETEX", key, exp, b)
	if err != nil {
		return errors.Wrap(err, "save frames error")
	}

	log.WithFields(log.Fields{
		"token":  frames.Token,
		"ctx_id": ctx.Value(logging.ContextIDKey),
	}).Info("downlink-frames saved")

	return nil
}

// GetDownlinkFrames returns the downlink-frames.
func GetDownlinkFrames(ctx context.Context, p *redis.Pool, token uint16) (DownlinkFrames, error) {
	c := p.Get()
	defer c.Close()

	key := fmt.Sprintf(downlinkFramesKeyTempl, token)
	val, err := redis.Bytes(c.Do("GET", key))
	if err != nil {
		if err == redis.ErrNil {
			return DownlinkFrames{}, ErrDoesNotExist
		}
	}

	var df DownlinkFrames
	err = proto.Unmarshal(val, &df)
	if err != nil {
		return df, errors.Wrap(err, "protobuf unmarshal error")
	}

	return df, nil
}
