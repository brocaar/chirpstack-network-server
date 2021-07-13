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
		for i := uint32(9); i < 31; i++ {
			if i < 13 {
				req.UplinkHistory = append(req.UplinkHistory, adr.UplinkMetaData{
					FCnt: i,
				})
				continue
			}

			if i == 13 || i == 19 {
				continue
			}

			req.UplinkHistory = append(req.UplinkHistory, adr.UplinkMetaData{
				FCnt: i,
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
				name:                 "two steps: two steps data-rate increase",
				nStep:                2,
				txPowerIndex:         0,
				dr:                   0,
				maxTxPowerIndex:      5,
				maxDR:                5,
				expectedDR:           2,
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

	t.Run("Handle", func(t *testing.T) {
		tests := []struct {
			name     string
			request  adr.HandleRequest
			response adr.HandleResponse
		}{
			{
				name: "increase dr",
				request: adr.HandleRequest{
					ADR:              true,
					DR:               0,
					TxPowerIndex:     0,
					NbTrans:          1,
					MaxDR:            5,
					MaxTxPowerIndex:  5,
					RequiredSNRForDR: 1,
					UplinkHistory: []adr.UplinkMetaData{
						{
							FCnt: 9,
							MaxSNR: 1,
						},
						{
							FCnt: 10,
							MaxSNR: 2,
						},
						{
							FCnt: 11,
							MaxSNR: 3,
						},
						{
							FCnt: 12,
							MaxSNR: 5,
						},
						{
							FCnt: 14,
							MaxSNR: 4,
						},
						{
							FCnt: 15,
							MaxSNR: 2,
						},
						{
							FCnt: 16,
							MaxSNR: 3,
						},
						{
							FCnt: 17,
							MaxSNR: 5,
						},
						{
							FCnt: 18,
							MaxSNR: 6,
						},
						{
							FCnt: 20,
							MaxSNR: 2,
						},
						{
							FCnt: 21,
							MaxSNR: 3,
						},
						{
							FCnt: 22,
							MaxSNR: 5,
						},
						{
							FCnt: 23,
							MaxSNR: 4,
						},
						{
							FCnt: 24,
							MaxSNR: 3,
						},
						{
							FCnt: 25,
							MaxSNR: 2,
						},
						{
							FCnt: 26,
							MaxSNR: 1,
						},
						{
							FCnt: 27,
							MaxSNR: 5,
						},
						{
							FCnt: 28,
							MaxSNR: 6,
						},
						{
							FCnt: 29,
							MaxSNR: 7,
						},
						{
							FCnt: 30,
							MaxSNR: 4,
						},
					},
				},
				response: adr.HandleResponse{
					DR:           1,
					TxPowerIndex: 0,
					NbTrans:      2,
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