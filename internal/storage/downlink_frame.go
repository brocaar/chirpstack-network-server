//go:generate protoc -I=/protobuf/src -I=/tmp/chirpstack-api/protobuf -I=. --go_out=. downlink_frame.proto

package storage

import (
	"context"
	"time"

	proto "github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/chirpstack-network-server/v3/internal/logging"
)

const downlinkFrameTTL = time.Second * 10
const downlinkFrameKeyTempl = "lora:ns:frame:%d"

// SaveDownlinkFrame saves the given downlink-frame.
func SaveDownlinkFrame(ctx context.Context, frame *DownlinkFrame) error {
	key := GetRedisKey(downlinkFrameKeyTempl, frame.Token)

	b, err := proto.Marshal(frame)
	if err != nil {
		return errors.Wrap(err, "marshal proto error")
	}

	err = RedisClient().Set(ctx, key, b, downlinkFrameTTL).Err()
	if err != nil {
		return errors.Wrap(err, "save downlink-frame error")
	}

	log.WithFields(log.Fields{
		"token":  frame.Token,
		"ctx_id": ctx.Value(logging.ContextIDKey),
	}).Info("storage: downlink-frame saved")

	return nil
}

// GetDownlinkFrame returns the downlink-frame matching the given token.
func GetDownlinkFrame(ctx context.Context, token uint16) (*DownlinkFrame, error) {
	key := GetRedisKey(downlinkFrameKeyTempl, token)

	val, err := RedisClient().Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, ErrDoesNotExist
		}
		return nil, errors.Wrap(err, "get downlink-frame error")
	}

	var df DownlinkFrame
	err = proto.Unmarshal(val, &df)
	if err != nil {
		return nil, errors.Wrap(err, "protobuf unmarshal error")
	}

	return &df, nil
}
