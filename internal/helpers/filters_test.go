package helpers

import (
	"testing"

	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/brocaar/chirpstack-network-server/internal/config"
	"github.com/brocaar/chirpstack-network-server/internal/models"
	"github.com/brocaar/lorawan"
	"github.com/gofrs/uuid"
	"github.com/stretchr/testify/require"
)

func TestFilterRxInfoByPublicOnly(t *testing.T) {
	tests := []struct {
		name            string
		forceGwsPrivate bool
		in              models.RXPacket
		expected        models.RXPacket
		expectedError   error
	}{
		{
			name: "one public gateway",
			in: models.RXPacket{
				RXInfoSet: []*gw.UplinkRXInfo{
					{
						GatewayId: []byte{1, 2, 3, 4, 5, 6, 7, 8},
					},
				},
				GatewayIsPrivate: map[lorawan.EUI64]bool{
					lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}: false,
				},
			},
			expected: models.RXPacket{
				RXInfoSet: []*gw.UplinkRXInfo{
					{
						GatewayId: []byte{1, 2, 3, 4, 5, 6, 7, 8},
					},
				},
				GatewayIsPrivate: map[lorawan.EUI64]bool{
					lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}: false,
				},
			},
		},
		{
			name: "one private gateway",
			in: models.RXPacket{
				RXInfoSet: []*gw.UplinkRXInfo{
					{
						GatewayId: []byte{1, 2, 3, 4, 5, 6, 7, 8},
					},
				},
				GatewayIsPrivate: map[lorawan.EUI64]bool{
					lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}: true,
				},
			},
			expectedError: ErrNoElements,
		},
		{
			name: "one private and one public gateway",
			in: models.RXPacket{
				RXInfoSet: []*gw.UplinkRXInfo{
					{
						GatewayId: []byte{1, 2, 3, 4, 5, 6, 7, 8},
					},
					{
						GatewayId: []byte{2, 2, 3, 4, 5, 6, 7, 8},
					},
				},
				GatewayIsPrivate: map[lorawan.EUI64]bool{
					lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}: true,
				},
			},
			expected: models.RXPacket{
				RXInfoSet: []*gw.UplinkRXInfo{
					{
						GatewayId: []byte{2, 2, 3, 4, 5, 6, 7, 8},
					},
				},
				GatewayIsPrivate: map[lorawan.EUI64]bool{
					lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}: true,
				},
			},
		},
		{
			name:            "one public gateway with gws forced to private",
			forceGwsPrivate: true,
			in: models.RXPacket{
				RXInfoSet: []*gw.UplinkRXInfo{
					{
						GatewayId: []byte{1, 2, 3, 4, 5, 6, 7, 8},
					},
				},
				GatewayIsPrivate: map[lorawan.EUI64]bool{
					lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}: false,
				},
			},
			expectedError: ErrNoElements,
		},
	}

	for _, tst := range tests {
		t.Run(tst.name, func(t *testing.T) {
			conf := config.Get()
			conf.NetworkServer.Gateway.ForceGwsPrivate = tst.forceGwsPrivate

			assert := require.New(t)
			err := FilterRxInfoByPublicOnly(&tst.in)
			assert.Equal(tst.expectedError, err)
			if tst.expectedError != nil {
				return
			}

			assert.Equal(tst.expected, tst.in)
		})
	}
}

func TestFilterRxInfoByServiceProfileID(t *testing.T) {
	serviceProfileID1, _ := uuid.NewV4()
	serviceProfileID2, _ := uuid.NewV4()

	tests := []struct {
		name             string
		forceGwsPrivate  bool
		serviceProfileID uuid.UUID
		in               models.RXPacket
		expected         models.RXPacket
		expectedError    error
	}{
		{
			name:             "one public gateway",
			serviceProfileID: serviceProfileID1,
			in: models.RXPacket{
				RXInfoSet: []*gw.UplinkRXInfo{
					{
						GatewayId: []byte{1, 2, 3, 4, 5, 6, 7, 8},
					},
				},
				GatewayIsPrivate: map[lorawan.EUI64]bool{
					lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}: false,
				},
			},
			expected: models.RXPacket{
				RXInfoSet: []*gw.UplinkRXInfo{
					{
						GatewayId: []byte{1, 2, 3, 4, 5, 6, 7, 8},
					},
				},
				GatewayIsPrivate: map[lorawan.EUI64]bool{
					lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}: false,
				},
			},
		},
		{
			name:             "one public gateway - gws forced to private",
			forceGwsPrivate:  true,
			serviceProfileID: serviceProfileID1,
			in: models.RXPacket{
				RXInfoSet: []*gw.UplinkRXInfo{
					{
						GatewayId: []byte{1, 2, 3, 4, 5, 6, 7, 8},
					},
				},
				GatewayIsPrivate: map[lorawan.EUI64]bool{
					lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}: false,
				},
			},
			expectedError: ErrNoElements,
		},
		{
			name:             "one private gateway, not matching service-profile id",
			serviceProfileID: serviceProfileID1,
			in: models.RXPacket{
				RXInfoSet: []*gw.UplinkRXInfo{
					{
						GatewayId: []byte{1, 2, 3, 4, 5, 6, 7, 8},
					},
				},
				GatewayIsPrivate: map[lorawan.EUI64]bool{
					lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}: true,
				},
				GatewayServiceProfile: map[lorawan.EUI64]uuid.UUID{
					lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}: serviceProfileID2,
				},
			},
			expectedError: ErrNoElements,
		},
		{
			name:             "one private gateway, matching service-profile id",
			serviceProfileID: serviceProfileID1,
			in: models.RXPacket{
				RXInfoSet: []*gw.UplinkRXInfo{
					{
						GatewayId: []byte{1, 2, 3, 4, 5, 6, 7, 8},
					},
				},
				GatewayIsPrivate: map[lorawan.EUI64]bool{
					lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}: true,
				},
				GatewayServiceProfile: map[lorawan.EUI64]uuid.UUID{
					lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}: serviceProfileID1,
				},
			},
			expected: models.RXPacket{
				RXInfoSet: []*gw.UplinkRXInfo{
					{
						GatewayId: []byte{1, 2, 3, 4, 5, 6, 7, 8},
					},
				},
				GatewayIsPrivate: map[lorawan.EUI64]bool{
					lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}: true,
				},
				GatewayServiceProfile: map[lorawan.EUI64]uuid.UUID{
					lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}: serviceProfileID1,
				},
			},
		},
		{
			name:             "one public + one private gateway, not service-profile id",
			serviceProfileID: serviceProfileID1,
			in: models.RXPacket{
				RXInfoSet: []*gw.UplinkRXInfo{
					{
						GatewayId: []byte{1, 2, 3, 4, 5, 6, 7, 8},
					},
					{
						GatewayId: []byte{2, 2, 3, 4, 5, 6, 7, 8},
					},
				},
				GatewayIsPrivate: map[lorawan.EUI64]bool{
					lorawan.EUI64{2, 2, 3, 4, 5, 6, 7, 8}: true,
				},
				GatewayServiceProfile: map[lorawan.EUI64]uuid.UUID{
					lorawan.EUI64{2, 2, 3, 4, 5, 6, 7, 8}: serviceProfileID2,
				},
			},
			expected: models.RXPacket{
				RXInfoSet: []*gw.UplinkRXInfo{
					{
						GatewayId: []byte{1, 2, 3, 4, 5, 6, 7, 8},
					},
				},
				GatewayIsPrivate: map[lorawan.EUI64]bool{
					lorawan.EUI64{2, 2, 3, 4, 5, 6, 7, 8}: true,
				},
				GatewayServiceProfile: map[lorawan.EUI64]uuid.UUID{
					lorawan.EUI64{2, 2, 3, 4, 5, 6, 7, 8}: serviceProfileID2,
				},
			},
		},
	}

	for _, tst := range tests {
		t.Run(tst.name, func(t *testing.T) {
			conf := config.Get()
			conf.NetworkServer.Gateway.ForceGwsPrivate = tst.forceGwsPrivate

			assert := require.New(t)
			err := FilterRxInfoByServiceProfileID(tst.serviceProfileID, &tst.in)
			assert.Equal(tst.expectedError, err)
			if tst.expectedError != nil {
				return
			}

			assert.Equal(tst.expected, tst.in)
		})
	}
}
