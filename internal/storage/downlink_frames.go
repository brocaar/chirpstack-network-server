//go:generate protoc -I=/protobuf/src -I=/tmp/chirpstack-api/protobuf -I=. --go_out=. downlink_frames.proto

package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis/v7"
	proto "github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/chirpstack-network-server/internal/logging"
)

const downlinkFramesTTL = time.Second * 10
const downlinkFramesKeyTempl = "lora:ns:frames:%d"

// SaveDownlinkFrames saves the given downlink-frames.
func SaveDownlinkFrames(ctx context.Context, frames DownlinkFrames) error {
	key := fmt.Sprintf(downlinkFramesKeyTempl, frames.Token)

	b, err := proto.Marshal(&frames)
	if err != nil {
		return errors.Wrap(err, "marshal proto error")
	}

	err = RedisClient().Set(key, b, downlinkFramesTTL).Err()
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
func GetDownlinkFrames(ctx context.Context, token uint16) (DownlinkFrames, error) {
	key := fmt.Sprintf(downlinkFramesKeyTempl, token)

	val, err := RedisClient().Get(key).Bytes()
	if err != nil {
		if err == redis.Nil {
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
