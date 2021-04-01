package adr

import (
	"fmt"
	"testing"

	"github.com/brocaar/chirpstack-network-server/v3/adr"
	"github.com/stretchr/testify/require"
)

func TestDefaultHandler(t *testing.T) {
	h := &DefaultHandler{}

	t.Run("ID", func(t *testing.T) {
		assert := require.New(t)
		id, err := h.ID()
		assert.NoError(err)
		assert.Equal("default", id)
	})

	t.Run("getPacketLossPercentage", func(t *testing.T) {
		assert := require.New(t)

		req := adr.HandleRequest{}
		for i := uint32(0); i < 20; i++ {
			if i < 5 {
				req.UplinkHistory = append(req.UplinkHistory, adr.UplinkMetaData{
					FCnt: i,
				})
				continue
			}

			if i < 10 {
				req.UplinkHistory = append(req.UplinkHistory, adr.UplinkMetaData{
					FCnt: i + 1,
				})
				continue
			}

			req.UplinkHistory = append(req.UplinkHistory, adr.UplinkMetaData{
				FCnt: i + 2,
			})
		}

		assert.EqualValues(10, h.getPacketLossPercentage(req))
	})

	t.Run("getNbTrans", func(t *testing.T) {
		tests := []struct {
			pktlossRate     float32
			currentNbTrans  int
			expectedNbTrant int
		}{
			{4.99, 3, 2},
			{9.99, 2, 2},
			{29.99, 1, 2},
			{30, 3, 3},
		}

		for _, tst := range tests {
			t.Run(fmt.Sprintf("packetloss rate: %f, current nbTrans: %d", tst.pktlossRate, tst.currentNbTrans), func(t *testing.T) {
				assert := require.New(t)
				assert.Equal(tst.expectedNbTrant, h.getNbTrans(tst.currentNbTrans, tst.pktlossRate))
			})
		}
	})

	t.Run("getIdealTxPowerIndexAndDR", func(t *testing.T) {
		tests := []struct {
			name                 string
			nStep                int
			txPowerIndex         int
			dr                   int
			maxTxPowerIndex      int
			maxDR                int
			expectedTxPowerIndex int
			expectedDR           int
		}{
			{
				name:                 "nothing to adjust",
				nStep:                0,
				txPowerIndex:         1,
				dr:                   3,
				maxTxPowerIndex:      5,
				maxDR:                5,
				expectedTxPowerIndex: 1,
				expectedDR:           3,
			},
			{
				name:                 "one step: one step data-rate increase",
				nStep:                1,
				txPowerIndex:         1,
				dr:                   4,
				maxTxPowerIndex:      5,
				maxDR:                5,
				expectedDR:           5,
				expectedTxPowerIndex: 1,
			},
			{
				name:                 "one step: one step tx-power decrease",
				nStep:                1,
				txPowerIndex:         1,
				dr:                   5,
				maxTxPowerIndex:      5,
				maxDR:                5,
				expectedDR:           5,
				expectedTxPowerIndex: 2,
			},
			{
				name:                 "two steps: two steps data-rate increase",
				nStep:                2,
				txPowerIndex:         1,
				dr:                   3,
				maxTxPowerIndex:      5,
				maxDR:                5,
				expectedDR:           5,
				expectedTxPowerIndex: 1,
			},
			{
				name:                 "two steps: one step data-rate increase, one step tx-power decrease",
				nStep:                2,
				txPowerIndex:         1,
				dr:                   4,
				maxTxPowerIndex:      5,
				maxDR:                5,
				expectedDR:           5,
				expectedTxPowerIndex: 2,
			},
			{
				name:                 "two step tx-power decrease",
				nStep:                2,
				txPowerIndex:         1,
				dr:                   5,
				maxTxPowerIndex:      5,
				maxDR:                5,
				expectedDR:           5,
				expectedTxPowerIndex: 3,
			},
			{
				name:                 "stwo steps: one step tx-power decrease",
				nStep:                2,
				txPowerIndex:         4,
				dr:                   5,
				maxTxPowerIndex:      5,
				maxDR:                5,
				expectedDR:           5,
				expectedTxPowerIndex: 5,
			},
			{
				name:                 "one negative step: one step power increase",
				nStep:                -1,
				txPowerIndex:         1,
				dr:                   5,
				maxTxPowerIndex:      5,
				maxDR:                5,
				expectedDR:           5,
				expectedTxPowerIndex: 0,
			},
			{
				name:                 "one negative step: nothing to do (adr engine will not decrease dr)",
				nStep:                -1,
				txPowerIndex:         0,
				dr:                   4,
				maxTxPowerIndex:      5,
				maxDR:                5,
				expectedDR:           4,
				expectedTxPowerIndex: 0,
			},
			{
				name:                 "10 negative steps, should not adjust anything (as we already reached the min tx-power index)",
				nStep:                -10,
				txPowerIndex:         0,
				dr:                   4,
				maxTxPowerIndex:      5,
				maxDR:                5,
				expectedDR:           4,
				expectedTxPowerIndex: 0,
			},
		}

		for _, tst := range tests {
			t.Run(tst.name, func(t *testing.T) {
				assert := require.New(t)
				txPowerIndex, dr := h.getIdealTxPowerIndexAndDR(tst.nStep, tst.txPowerIndex, tst.dr, tst.maxTxPowerIndex, tst.maxDR)
				assert.Equal(tst.expectedDR, dr)
				assert.Equal(tst.expectedTxPowerIndex, txPowerIndex)
			})
		}
	})

	t.Run("requiredHistoryCount", func(t *testing.T) {
		assert := require.New(t)
		assert.Equal(20, h.requiredHistoryCount())
	})

	t.Run("getMaxSNR", func(t *testing.T) {
		assert := require.New(t)

		req := adr.HandleRequest{
			UplinkHistory: []adr.UplinkMetaData{
				{MaxSNR: 3},
				{MaxSNR: 4},
				{MaxSNR: 2},
			},
		}

		assert.EqualValues(4, h.getMaxSNR(req))
	})

	t.Run("Handle", func(t *testing.T) {
		tests := []struct {
			name     string
			request  adr.HandleRequest
			response adr.HandleResponse
		}{
			{
				name: "max dr exceeded, adr disabled",
				request: adr.HandleRequest{
					ADR:             false,
					DR:              5,
					TxPowerIndex:    0,
					NbTrans:         1,
					MaxDR:           4,
					MaxTxPowerIndex: 5,
				},
				response: adr.HandleResponse{
					DR:           5,
					TxPowerIndex: 0,
					NbTrans:      1,
				},
			},
			{
				name: "max dr exceeded, decrease dr",
				request: adr.HandleRequest{
					ADR:             true,
					DR:              5,
					TxPowerIndex:    0,
					NbTrans:         1,
					MaxDR:           4,
					MaxTxPowerIndex: 5,
					UplinkHistory: []adr.UplinkMetaData{
						{
							MaxSNR: 0,
						},
					},
				},
				response: adr.HandleResponse{
					DR:           4,
					TxPowerIndex: 0,
					NbTrans:      1,
				},
			},
			{
				name: "increase dr",
				request: adr.HandleRequest{
					ADR:              true,
					DR:               0,
					TxPowerIndex:     0,
					NbTrans:          1,
					MaxDR:            5,
					MaxTxPowerIndex:  5,
					RequiredSNRForDR: -20,
					UplinkHistory: []adr.UplinkMetaData{
						{
							MaxSNR: -15,
						},
					},
				},
				response: adr.HandleResponse{
					DR:           1,
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
