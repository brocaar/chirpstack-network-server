package gateway

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/brocaar/chirpstack-network-server/internal/band"
	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/brocaar/chirpstack-network-server/internal/test"
	"github.com/brocaar/lorawan"
)

func TestSelectDownlinkGateway(t *testing.T) {
	assert := require.New(t)

	config := test.GetConfig()
	assert.NoError(band.Setup(config))

	tests := []struct {
		Name          string
		MinSNRMargin  float64
		DR            int
		RxInfo        []storage.DeviceGatewayRXInfo
		ExpectedIn    []storage.DeviceGatewayRXInfo
		ExpectedError error
	}{
		{
			Name:          "empty",
			ExpectedError: errors.New("device gateway rx-info slice is empty"),
		},
		{
			Name: "single rx info",
			RxInfo: []storage.DeviceGatewayRXInfo{
				{
					GatewayID: lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 1},
					LoRaSNR:   -5,
				},
			},
			ExpectedIn: []storage.DeviceGatewayRXInfo{
				{
					GatewayID: lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 1},
					LoRaSNR:   -5,
				},
			},
		},
		{
			Name: "two items, both below MinSNR",
			DR:   2, // -15 is required
			RxInfo: []storage.DeviceGatewayRXInfo{
				{
					GatewayID: lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 1},
					LoRaSNR:   -12,
				},
				{
					GatewayID: lorawan.EUI64{2, 2, 2, 2, 2, 2, 2, 2},
					LoRaSNR:   -11,
				},
			},
			MinSNRMargin: 5,
			ExpectedIn: []storage.DeviceGatewayRXInfo{
				{
					GatewayID: lorawan.EUI64{2, 2, 2, 2, 2, 2, 2, 2},
					LoRaSNR:   -11,
				},
			},
		},
		{
			Name: "two items, one below MinSNR",
			DR:   2, // -15 is required
			RxInfo: []storage.DeviceGatewayRXInfo{
				{
					GatewayID: lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 1},
					LoRaSNR:   -12,
				},
				{
					GatewayID: lorawan.EUI64{2, 2, 2, 2, 2, 2, 2, 2},
					LoRaSNR:   -10,
				},
			},
			MinSNRMargin: 5,
			ExpectedIn: []storage.DeviceGatewayRXInfo{
				{
					GatewayID: lorawan.EUI64{2, 2, 2, 2, 2, 2, 2, 2},
					LoRaSNR:   -10,
				},
			},
		},
		{
			Name: "four items, two below MinSNR",
			DR:   2, // -15 is required
			RxInfo: []storage.DeviceGatewayRXInfo{
				{
					GatewayID: lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 1},
					LoRaSNR:   -12,
				},
				{
					GatewayID: lorawan.EUI64{2, 2, 2, 2, 2, 2, 2, 2},
					LoRaSNR:   -11,
				},
				{
					GatewayID: lorawan.EUI64{3, 3, 3, 3, 3, 3, 3, 3},
					LoRaSNR:   -10,
				},
				{
					GatewayID: lorawan.EUI64{4, 4, 4, 4, 4, 4, 4, 4},
					LoRaSNR:   -9,
				},
			},
			MinSNRMargin: 5,
			ExpectedIn: []storage.DeviceGatewayRXInfo{
				{
					GatewayID: lorawan.EUI64{3, 3, 3, 3, 3, 3, 3, 3},
					LoRaSNR:   -10,
				},
				{
					GatewayID: lorawan.EUI64{4, 4, 4, 4, 4, 4, 4, 4},
					LoRaSNR:   -9,
				},
			},
		},
	}

	for _, tst := range tests {
		t.Run(tst.Name, func(t *testing.T) {
			assert := require.New(t)

			if len(tst.ExpectedIn) > 1 {
				outMap := make(map[lorawan.EUI64]struct{})

				for i := 0; i < 100*len(tst.ExpectedIn); i++ {
					out, err := SelectDownlinkGateway(tst.MinSNRMargin, tst.DR, tst.RxInfo)
					if tst.ExpectedError != nil {
						assert.Equal(tst.ExpectedError.Error(), err.Error())
						return
					}
					assert.NoError(err)
					assert.Contains(tst.ExpectedIn, out)
					outMap[out.GatewayID] = struct{}{}
				}

				// assert that we did receive different values
				assert.Equal(len(tst.ExpectedIn), len(outMap))
			}

		})
	}
}
