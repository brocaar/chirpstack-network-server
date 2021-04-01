package maccommand

import (
	"context"
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/stretchr/testify/require"

	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/brocaar/chirpstack-network-server/v3/internal/gps"
	"github.com/brocaar/chirpstack-network-server/v3/internal/models"
	"github.com/brocaar/chirpstack-network-server/v3/internal/storage"
	"github.com/brocaar/lorawan"
)

func TestDeviceTime(t *testing.T) {
	now := time.Now().Add(time.Minute)
	nowPB, _ := ptypes.TimestampProto(now)
	timeSinceGPSEpoch := time.Minute
	timeSinceGPSEpochPB := ptypes.DurationProto(timeSinceGPSEpoch)

	tests := []struct {
		Name               string
		RXPacket           models.RXPacket
		ExpectedMACCommand lorawan.MACCommand
	}{
		{
			Name: "time since gps epoch",
			RXPacket: models.RXPacket{
				RXInfoSet: []*gw.UplinkRXInfo{
					{
						TimeSinceGpsEpoch: timeSinceGPSEpochPB,
						Time:              nowPB,
					},
				},
			},
			ExpectedMACCommand: lorawan.MACCommand{
				CID: lorawan.DeviceTimeAns,
				Payload: &lorawan.DeviceTimeAnsPayload{
					TimeSinceGPSEpoch: timeSinceGPSEpoch,
				},
			},
		},
		{
			Name: "time field",
			RXPacket: models.RXPacket{
				RXInfoSet: []*gw.UplinkRXInfo{
					{
						Time: nowPB,
					},
				},
			},
			ExpectedMACCommand: lorawan.MACCommand{
				CID: lorawan.DeviceTimeAns,
				Payload: &lorawan.DeviceTimeAnsPayload{
					TimeSinceGPSEpoch: gps.Time(now).TimeSinceGPSEpoch(),
				},
			},
		},
	}

	for _, tst := range tests {
		t.Run(tst.Name, func(t *testing.T) {
			assert := require.New(t)

			resp, err := handleDeviceTimeReq(context.Background(), &storage.DeviceSession{}, tst.RXPacket)
			assert.NoError(err)
			assert.Len(resp, 1)
			assert.Len(resp[0].MACCommands, 1)
			assert.Equal(tst.ExpectedMACCommand, resp[0].MACCommands[0])
		})
	}

	t.Run("server time", func(t *testing.T) {
		assert := require.New(t)

		rxPacket := models.RXPacket{
			RXInfoSet: []*gw.UplinkRXInfo{
				{},
			},
		}

		resp, err := handleDeviceTimeReq(context.Background(), &storage.DeviceSession{}, rxPacket)
		assert.NoError(err)
		assert.Len(resp, 1)
		assert.Len(resp[0].MACCommands, 1)

		pl, ok := resp[0].MACCommands[0].Payload.(*lorawan.DeviceTimeAnsPayload)
		assert.True(ok)

		assert.NotEqual(timeSinceGPSEpoch, pl.TimeSinceGPSEpoch)
		assert.NotEqual(gps.Time(now).TimeSinceGPSEpoch(), pl.TimeSinceGPSEpoch)
		assert.InDelta(gps.Time(time.Now()).TimeSinceGPSEpoch(), pl.TimeSinceGPSEpoch, float64(time.Second))
	})
}
