//go:generate protoc -I=/protobuf/src -I=/tmp/chirpstack-api/protobuf -I=. --go_out=. downlink_frame.proto

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

const downlinkFrameTTL = time.Second * 10
const downlinkFrameKeyTempl = "lora:ns:frame:%d"

// SaveDownlinkFrame saves the given downlink-frame.
func SaveDownlinkFrame(ctx context.Context, frame DownlinkFrame) error {
	key := fmt.Sprintf(downlinkFrameKeyTempl, frame.Token)

	b, err := proto.Marshal(&frame)
	if err != nil {
		return errors.Wrap(err, "marshal proto error")
	}

	err = RedisClient().Set(key, b, downlinkFrameTTL).Err()
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
func GetDownlinkFrame(ctx context.Context, token uint16) (DownlinkFrame, error) {
	key := fmt.Sprintf(downlinkFrameKeyTempl, token)

	val, err := RedisClient().Get(key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return DownlinkFrame{}, ErrDoesNotExist
		}
		return DownlinkFrame{}, errors.Wrap(err, "get downlink-frame error")
	}

	var df DownlinkFrame
	err = proto.Unmarshal(val, &df)
	if err != nil {
		return df, errors.Wrap(err, "protobuf unmarshal error")
	}

	return df, nil
}
