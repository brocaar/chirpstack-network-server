package adr

import (
	"testing"

	"github.com/brocaar/chirpstack-network-server/v3/adr"
	"github.com/brocaar/chirpstack-network-server/v3/internal/band"
	"github.com/brocaar/chirpstack-network-server/v3/internal/test"
	"github.com/stretchr/testify/require"
)

func TestLoRaLRFHSSHandler(t *testing.T) {
	h := &LoRaLRFHSSHandler{}

	t.Run("ID", func(t *testing.T) {
		assert := require.New(t)
		id, err := h.ID()
		assert.NoError(err)
		assert.Equal("lora_lr_fhss", id)
	})

	t.Run("Handle", func(t *testing.T) {
		conf := test.GetConfig()

		// Add channel with LR-FHSS data-rate enabled.
		conf.NetworkServer.NetworkSettings.ExtraChannels = append(conf.NetworkServer.NetworkSettings.ExtraChannels, struct {
			Frequency uint32 `mapstructure:"frequency"`
			MinDR     int    `mapstructure:"min_dr"`
			MaxDR     int    `mapstructure:"max_dr"`
		}{
			Frequency: 867300000,
			MinDR:     10,
			MaxDR:     11,
		})
		band.Setup(conf)

		tests := []struct {
			name     string
			request  adr.HandleRequest
			response adr.HandleResponse
		}{
			{
				name: "switch to DR 3 (LoRa)",
				request: adr.HandleRequest{
					ADR:              true,
					DR:               0,
					NbTrans:          1,
					MaxDR:            11,
					RequiredSNRForDR: -20,
					UplinkHistory: []adr.UplinkMetaData{
						{
							MaxSNR: -10,
						},
					},
				},
				response: adr.HandleResponse{
					DR:      3,
					NbTrans: 1,
				},
			},
			{
				name: "switch to DR 3 (LoRa)",
				request: adr.HandleRequest{
					ADR:              true,
					DR:               0,
					NbTrans:          3,
					MaxDR:            11,
					RequiredSNRForDR: -20,
					UplinkHistory: []adr.UplinkMetaData{
						{
							MaxSNR: -12,
						},
					},
				},
				response: adr.HandleResponse{
					DR:      10,
					NbTrans: 1,
				},
			},
		}

		for _, tst := range tests {
			t.Run(tst.name, func(t *testing.T) {
				assert := require.New(t)

				resp, err := h.Handle(tst.request)
				assert.NoError(err)
				assert.Equal(tst.response, resp)
			})
		}
	})
}
