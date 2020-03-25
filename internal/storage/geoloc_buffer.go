package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/pkg/errors"

	"github.com/brocaar/chirpstack-api/go/v3/geo"
	"github.com/brocaar/lorawan"
)

const (
	geolocBufferKeyTempl = "lora:ns:device:%s:geoloc:buffer"
)

// SaveGeolocBuffer saves the given items in the geolocation buffer.
// It overwrites the previous buffer to make sure that expired items do not
// stay in the buffer as the TTL is set on the key, not on the items.
func SaveGeolocBuffer(ctx context.Context, devEUI lorawan.EUI64, items []*geo.FrameRXInfo, ttl time.Duration) error {
	// nothing to do
	if ttl == 0 || len(items) == 0 {
		return nil
	}

	key := fmt.Sprintf(geolocBufferKeyTempl, devEUI)

	pipe := RedisClient().TxPipeline()

	pipe.Del(key)
	for _, item := range items {
		b, err := proto.Marshal(item)
		if err != nil {
			return errors.Wrap(err, "protobuf marshal error")
		}

		pipe.RPush(key, b)
	}
	pipe.PExpire(key, ttl)

	if _, err := pipe.Exec(); err != nil {
		return errors.Wrap(err, "redis exec error")
	}

	return nil
}

// GetGeolocBuffer returns the geolocation buffer. Items that exceed the
// given TTL are not returned.
func GetGeolocBuffer(ctx context.Context, devEUI lorawan.EUI64, ttl time.Duration) ([]*geo.FrameRXInfo, error) {
	// nothing to do
	if ttl == 0 {
		return nil, nil
	}

	key := fmt.Sprintf(geolocBufferKeyTempl, devEUI)
	resp, err := RedisClient().LRange(key, 0, -1).Result()
	if err != nil {
		return nil, errors.Wrap(err, "read buffer error")
	}

	out := make([]*geo.FrameRXInfo, 0, len(resp))

	for _, b := range resp {
		var item geo.FrameRXInfo
		if err := proto.Unmarshal([]byte(b), &item); err != nil {
			return nil, errors.Wrap(err, "protobuf unmarshal error")
		}

		add := true

		for _, rxInfo := range item.RxInfo {
			// Ignore frames without time which could happen when a gateway
			// for example lost its gps fix, ...
			// Avoid that a missing Time results in an error in the next step.
			if rxInfo.Time == nil {
				add = false
				break
			}

			ts, err := ptypes.Timestamp(rxInfo.Time)
			if err != nil {
				return nil, errors.Wrap(err, "get timestamp error")
			}

			// Ignore items before TTL as the TTL is set on the key of the buffer,
			// not on the item.
			if time.Now().Sub(ts) > ttl {
				add = false
				break
			}
		}

		if add {
			out = append(out, &item)
		}
	}

	return out, nil
}
