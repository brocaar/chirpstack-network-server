package multicast

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/band"
)

func TestGetMinimumGatewaySet(t *testing.T) {
	config.C.NetworkServer.Band.Band, _ = band.GetConfig(band.EU_863_870, false, lorawan.DwellTimeNoLimit)

	testTable := []struct {
		Name             string
		RxInfoSets       []storage.DeviceGatewayRXInfoSet
		ExpectedGateways []lorawan.EUI64
	}{
		{
			Name: "one device - one gateway",
			RxInfoSets: []storage.DeviceGatewayRXInfoSet{
				{
					DevEUI: lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 1},
					Items: []storage.DeviceGatewayRXInfo{
						{
							GatewayID: lorawan.EUI64{2, 2, 2, 2, 2, 2, 2, 1},
							LoRaSNR:   5,
						},
					},
				},
			},
			ExpectedGateways: []lorawan.EUI64{{2, 2, 2, 2, 2, 2, 2, 1}},
		},
		{
			Name: "one device - two gateways",
			RxInfoSets: []storage.DeviceGatewayRXInfoSet{
				{
					DevEUI: lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 1},
					Items: []storage.DeviceGatewayRXInfo{
						{
							GatewayID: lorawan.EUI64{2, 2, 2, 2, 2, 2, 2, 1},
							LoRaSNR:   -21,
						},
						{
							GatewayID: lorawan.EUI64{2, 2, 2, 2, 2, 2, 2, 2},
							LoRaSNR:   -20,
						},
					},
				},
			},

			// as the first gateway does not meet the min. required SNR.
			ExpectedGateways: []lorawan.EUI64{{2, 2, 2, 2, 2, 2, 2, 2}},
		},
		{
			Name: "two devices - two gateways (no overlap)",
			RxInfoSets: []storage.DeviceGatewayRXInfoSet{
				{
					DevEUI: lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 1},
					Items: []storage.DeviceGatewayRXInfo{
						{
							GatewayID: lorawan.EUI64{2, 2, 2, 2, 2, 2, 2, 1},
							LoRaSNR:   5,
						},
					},
				},
				{
					DevEUI: lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 2},
					Items: []storage.DeviceGatewayRXInfo{
						{
							GatewayID: lorawan.EUI64{2, 2, 2, 2, 2, 2, 2, 2},
							LoRaSNR:   5,
						},
					},
				},
			},
			ExpectedGateways: []lorawan.EUI64{{2, 2, 2, 2, 2, 2, 2, 1}, {2, 2, 2, 2, 2, 2, 2, 2}},
		},
		{
			Name: "two devices - two gateways (overlap, first gw covers two devices)",
			RxInfoSets: []storage.DeviceGatewayRXInfoSet{
				{
					DevEUI: lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 1},
					Items: []storage.DeviceGatewayRXInfo{
						{
							GatewayID: lorawan.EUI64{2, 2, 2, 2, 2, 2, 2, 1},
							LoRaSNR:   5,
						},
					},
				},
				{
					DevEUI: lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 2},
					Items: []storage.DeviceGatewayRXInfo{
						{
							GatewayID: lorawan.EUI64{2, 2, 2, 2, 2, 2, 2, 1},
							LoRaSNR:   5,
						},
						{
							GatewayID: lorawan.EUI64{2, 2, 2, 2, 2, 2, 2, 2},
							LoRaSNR:   5,
						},
					},
				},
			},
			ExpectedGateways: []lorawan.EUI64{{2, 2, 2, 2, 2, 2, 2, 1}},
		},
		{
			Name: "two devices - two gateways (overlap, second gw covers two devices)",
			RxInfoSets: []storage.DeviceGatewayRXInfoSet{
				{
					DevEUI: lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 1},
					Items: []storage.DeviceGatewayRXInfo{
						{
							GatewayID: lorawan.EUI64{2, 2, 2, 2, 2, 2, 2, 2},
							LoRaSNR:   5,
						},
					},
				},
				{
					DevEUI: lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 2},
					Items: []storage.DeviceGatewayRXInfo{
						{
							GatewayID: lorawan.EUI64{2, 2, 2, 2, 2, 2, 2, 1},
							LoRaSNR:   5,
						},
						{
							GatewayID: lorawan.EUI64{2, 2, 2, 2, 2, 2, 2, 2},
							LoRaSNR:   5,
						},
					},
				},
			},
			ExpectedGateways: []lorawan.EUI64{{2, 2, 2, 2, 2, 2, 2, 2}},
		},
	}

	for _, test := range testTable {
		t.Run(test.Name, func(t *testing.T) {
			assert := require.New(t)

			gws, err := GetMinimumGatewaySet(test.RxInfoSets)
			assert.NoError(err)
			assert.ElementsMatch(gws, test.ExpectedGateways)
		})
	}
}
