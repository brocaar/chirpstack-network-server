package adr

import (
	"testing"

	"github.com/brocaar/chirpstack-network-server/v3/adr"
	"github.com/brocaar/chirpstack-network-server/v3/internal/band"
	"github.com/brocaar/chirpstack-network-server/v3/internal/test"
	"github.com/stretchr/testify/require"
)

func TestLRFHSSHandler(t *testing.T) {
	h := &LRFHSSHandler{}

	t.Run("ID", func(t *testing.T) {
		assert := require.New(t)
		id, err := h.ID()
		assert.NoError(err)
		assert.Equal("lr_fhss", id)
	})

	t.Run("Handle without LR-FHSS enabled", func(t *testing.T) {
		assert := require.New(t)
		conf := test.GetConfig()
		band.Setup(conf)

		resp, err := h.Handle(adr.HandleRequest{
			ADR:   true,
			DR:    1,
			MaxDR: 11,
		})
		assert.NoError(err)
		assert.Equal(adr.HandleResponse{
			DR: 1,
		}, resp)
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

		fullHistory := make([]adr.UplinkMetaData, 0, 20)
		for i := 0; i < 20; i++ {
			fullHistory = append(fullHistory, adr.UplinkMetaData{
				MaxRSSI: -130,
			})
		}

		tests := []struct {
			name     string
			request  adr.HandleRequest
			response adr.HandleResponse
		}{
			{
				name: "adr disabled",
				request: adr.HandleRequest{
					ADR:     false,
					DR:      0,
					NbTrans: 3,
					MaxDR:   11,
					UplinkHistory: []adr.UplinkMetaData{
						{
							MaxRSSI: -130,
						},
					},
				},
				response: adr.HandleResponse{
					DR:      0,
					NbTrans: 3,
				},
			},
			{
				name: "max dr. prevents lr-fhss",
				request: adr.HandleRequest{
					ADR:     false,
					DR:      0,
					NbTrans: 3,
					MaxDR:   5,
					UplinkHistory: []adr.UplinkMetaData{
						{
							MaxRSSI: -130,
						},
					},
				},
				response: adr.HandleResponse{
					DR:      0,
					NbTrans: 3,
				},
			},
			{
				name: "switch to dr 10",
				request: adr.HandleRequest{
					ADR:     true,
					DR:      0,
					NbTrans: 3,
					MaxDR:   11,
					UplinkHistory: []adr.UplinkMetaData{
						{
							MaxRSSI: -130,
						},
					},
				},
				response: adr.HandleResponse{
					DR:           10,
					TxPowerIndex: 0,
					NbTrans:      1,
				},
			},
			{
				name: "switch to dr 11",
				request: adr.HandleRequest{
					ADR:           true,
					DR:            0,
					NbTrans:       3,
					MaxDR:         11,
					UplinkHistory: fullHistory,
				},
				response: adr.HandleResponse{
					DR:           11,
					TxPowerIndex: 0,
					NbTrans:      1,
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
