package storage

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/brocaar/chirpstack-api/go/geo"
	"github.com/brocaar/chirpstack-api/go/gw"
	"github.com/brocaar/chirpstack-network-server/internal/test"
	"github.com/brocaar/lorawan"
	"github.com/golang/protobuf/ptypes"
	"github.com/stretchr/testify/require"
)

func (ts *StorageTestSuite) TestGeolocBuffer() {
	now, _ := ptypes.TimestampProto(time.Now())
	tenMinAgo, _ := ptypes.TimestampProto(time.Now().Add(-10 * time.Minute))

	tests := []struct {
		Name          string
		Items         []*geo.FrameRXInfo
		DevEUI        lorawan.EUI64
		AddTTL        time.Duration
		GetTTL        time.Duration
		ExpectedItems []*geo.FrameRXInfo
	}{
		{
			Name: "add one item to queue - ttl 0",
			Items: []*geo.FrameRXInfo{
				{
					RxInfo: []*gw.UplinkRXInfo{
						{
							Time: now,
						},
					},
				},
			},
		},
		{
			Name: "add one item to queue",
			Items: []*geo.FrameRXInfo{
				{
					RxInfo: []*gw.UplinkRXInfo{
						{
							Time: now,
						},
					},
				},
			},
			ExpectedItems: []*geo.FrameRXInfo{
				{
					RxInfo: []*gw.UplinkRXInfo{
						{
							Time: now,
						},
					},
				},
			},
			AddTTL: time.Minute,
			GetTTL: time.Minute,
		},
		{
			Name: "add three to queue, one expired",
			Items: []*geo.FrameRXInfo{
				{
					RxInfo: []*gw.UplinkRXInfo{
						{
							Time: now,
						},
					},
				},
				{
					RxInfo: []*gw.UplinkRXInfo{
						{
							Time: now,
						},
					},
				},
				{
					RxInfo: []*gw.UplinkRXInfo{
						{
							Time: tenMinAgo,
						},
					},
				},
			},
			ExpectedItems: []*geo.FrameRXInfo{
				{
					RxInfo: []*gw.UplinkRXInfo{
						{
							Time: now,
						},
					},
				},
				{
					RxInfo: []*gw.UplinkRXInfo{
						{
							Time: now,
						},
					},
				},
			},
			AddTTL: time.Minute,
			GetTTL: time.Minute,
		},
	}

	for _, tst := range tests {
		ts.T().Run(tst.Name, func(t *testing.T) {
			test.MustFlushRedis(RedisPool())
			assert := require.New(t)

			assert.NoError(SaveGeolocBuffer(context.Background(), RedisPool(), tst.DevEUI, tst.Items, tst.AddTTL))

			resp, err := GetGeolocBuffer(context.Background(), RedisPool(), tst.DevEUI, tst.GetTTL)
			assert.NoError(err)

			aa, _ := json.Marshal(tst.ExpectedItems)
			bb, _ := json.Marshal(resp)

			assert.Equal(string(aa), string(bb))
		})
	}
}
