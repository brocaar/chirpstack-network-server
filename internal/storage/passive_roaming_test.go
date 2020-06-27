package storage

import (
	"context"
	"testing"
	"time"

	"github.com/brocaar/lorawan"
	"github.com/gofrs/uuid"
	"github.com/stretchr/testify/require"
)

func (ts *StorageTestSuite) TestPassiveRoaming() {
	ts.T().Run("Save", func(t *testing.T) {
		assert := require.New(t)

		sessID, err := uuid.NewV4()
		assert.NoError(err)
		lt := time.Now().Add(time.Millisecond * 100).UTC()

		ds := PassiveRoamingDeviceSession{
			SessionID:   sessID,
			NetID:       lorawan.NetID{1, 2, 3},
			DevAddr:     lorawan.DevAddr{1, 2, 3, 4},
			DevEUI:      lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
			LoRaWAN11:   true,
			FNwkSIntKey: lorawan.AES128Key{1, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8},
			Lifetime:    lt,
			FCntUp:      10,
		}

		assert.NoError(SavePassiveRoamingDeviceSession(context.Background(), &ds))

		ts.T().Run("Get IDs for DevAddr", func(t *testing.T) {
			assert := require.New(t)

			ids, err := GetPassiveRoamingIDsForDevAddr(context.Background(), ds.DevAddr)
			assert.NoError(err)
			assert.Equal([]uuid.UUID{ds.SessionID}, ids)
		})

		ts.T().Run("Get for DevAddr", func(t *testing.T) {
			assert := require.New(t)

			sessions, err := GetPassiveRoamingDeviceSessionsForDevAddr(context.Background(), ds.DevAddr)
			assert.NoError(err)
			assert.Equal([]PassiveRoamingDeviceSession{ds}, sessions)
		})

		ts.T().Run("Get by ID", func(t *testing.T) {
			assert := require.New(t)

			dsGet, err := GetPassiveRoamingDeviceSession(context.Background(), sessID)
			assert.NoError(err)
			assert.Equal(ds, dsGet)
		})

		ts.T().Run("Expire", func(t *testing.T) {
			assert := require.New(t)
			time.Sleep(time.Millisecond * 100)

			_, err := GetPassiveRoamingDeviceSession(context.Background(), sessID)
			assert.Equal(ErrDoesNotExist, err)
		})
	})

	ts.T().Run("Get for PHYPayload", func(t *testing.T) {
		assert := require.New(t)
		id1, err := uuid.NewV4()
		assert.NoError(err)
		id2, err := uuid.NewV4()
		assert.NoError(err)

		fNwkSIntKey := lorawan.AES128Key{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}
		sNwkSIntKey := lorawan.AES128Key{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 2}

		lw10PHY := lorawan.PHYPayload{
			MHDR: lorawan.MHDR{
				MType: lorawan.UnconfirmedDataUp,
				Major: lorawan.LoRaWANR1,
			},
			MACPayload: &lorawan.MACPayload{
				FHDR: lorawan.FHDR{
					DevAddr: lorawan.DevAddr{1, 2, 3, 4},
					FCnt:    10,
				},
			},
		}
		assert.NoError(lw10PHY.SetUplinkDataMIC(lorawan.LoRaWAN1_0, 0, 0, 0, fNwkSIntKey, fNwkSIntKey))

		lw11PHY := lorawan.PHYPayload{
			MHDR: lorawan.MHDR{
				MType: lorawan.UnconfirmedDataUp,
				Major: lorawan.LoRaWANR1,
			},
			MACPayload: &lorawan.MACPayload{
				FHDR: lorawan.FHDR{
					DevAddr: lorawan.DevAddr{1, 2, 3, 4},
					FCnt:    10,
				},
			},
		}
		assert.NoError(lw11PHY.SetUplinkDataMIC(lorawan.LoRaWAN1_1, 100, 3, 7, fNwkSIntKey, sNwkSIntKey))

		tests := []struct {
			name             string
			storedSessions   []PassiveRoamingDeviceSession
			expectedSessions []PassiveRoamingDeviceSession
			phyPayload       lorawan.PHYPayload
		}{
			{
				name: "no mic check",
				storedSessions: []PassiveRoamingDeviceSession{
					{
						SessionID:   id1,
						DevAddr:     lorawan.DevAddr{1, 2, 3, 4},
						LoRaWAN11:   false,
						Lifetime:    time.Now().Add(time.Minute),
						FCntUp:      10,
						ValidateMIC: false,
					},
				},
				expectedSessions: []PassiveRoamingDeviceSession{
					{
						SessionID:   id1,
						DevAddr:     lorawan.DevAddr{1, 2, 3, 4},
						LoRaWAN11:   false,
						Lifetime:    time.Now().Add(time.Minute),
						FCntUp:      10,
						ValidateMIC: false,
					},
				},
				phyPayload: lw10PHY,
			},
			{
				name: "no mic check - multiple",
				storedSessions: []PassiveRoamingDeviceSession{
					{
						SessionID:   id1,
						DevAddr:     lorawan.DevAddr{1, 2, 3, 4},
						LoRaWAN11:   false,
						Lifetime:    time.Now().Add(time.Minute),
						FCntUp:      10,
						ValidateMIC: false,
					},
					{
						SessionID:   id2,
						DevAddr:     lorawan.DevAddr{1, 2, 3, 4},
						LoRaWAN11:   false,
						Lifetime:    time.Now().Add(time.Minute),
						FCntUp:      10,
						ValidateMIC: false,
					},
				},
				expectedSessions: []PassiveRoamingDeviceSession{
					{
						SessionID:   id1,
						DevAddr:     lorawan.DevAddr{1, 2, 3, 4},
						LoRaWAN11:   false,
						Lifetime:    time.Now().Add(time.Minute),
						FCntUp:      10,
						ValidateMIC: false,
					},
					{
						SessionID:   id2,
						DevAddr:     lorawan.DevAddr{1, 2, 3, 4},
						LoRaWAN11:   false,
						Lifetime:    time.Now().Add(time.Minute),
						FCntUp:      10,
						ValidateMIC: false,
					},
				},
				phyPayload: lw10PHY,
			},
			{
				name: "mic check LW10",
				storedSessions: []PassiveRoamingDeviceSession{
					{
						SessionID:   id1,
						DevAddr:     lorawan.DevAddr{1, 2, 3, 4},
						LoRaWAN11:   false,
						Lifetime:    time.Now().Add(time.Minute),
						FCntUp:      10,
						ValidateMIC: true,
						FNwkSIntKey: fNwkSIntKey,
					},
				},
				expectedSessions: []PassiveRoamingDeviceSession{
					{
						SessionID:   id1,
						DevAddr:     lorawan.DevAddr{1, 2, 3, 4},
						LoRaWAN11:   false,
						Lifetime:    time.Now().Add(time.Minute),
						FCntUp:      10,
						ValidateMIC: true,
						FNwkSIntKey: fNwkSIntKey,
					},
				},
				phyPayload: lw10PHY,
			},
			{
				name: "mic check LW10 - matching single out of two",
				storedSessions: []PassiveRoamingDeviceSession{
					{
						SessionID:   id1,
						DevAddr:     lorawan.DevAddr{1, 2, 3, 4},
						LoRaWAN11:   false,
						Lifetime:    time.Now().Add(time.Minute),
						FCntUp:      10,
						ValidateMIC: true,
					},
					{
						SessionID:   id2,
						DevAddr:     lorawan.DevAddr{1, 2, 3, 4},
						LoRaWAN11:   false,
						Lifetime:    time.Now().Add(time.Minute),
						FCntUp:      10,
						ValidateMIC: true,
						FNwkSIntKey: fNwkSIntKey,
					},
				},
				expectedSessions: []PassiveRoamingDeviceSession{
					{
						SessionID:   id2,
						DevAddr:     lorawan.DevAddr{1, 2, 3, 4},
						LoRaWAN11:   false,
						Lifetime:    time.Now().Add(time.Minute),
						FCntUp:      10,
						ValidateMIC: true,
						FNwkSIntKey: fNwkSIntKey,
					},
				},
				phyPayload: lw10PHY,
			},
			{
				name: "mic check LW10 - failing fcnt check",
				storedSessions: []PassiveRoamingDeviceSession{
					{
						SessionID:   id1,
						DevAddr:     lorawan.DevAddr{1, 2, 3, 4},
						LoRaWAN11:   false,
						Lifetime:    time.Now().Add(time.Minute),
						FCntUp:      20,
						ValidateMIC: true,
						FNwkSIntKey: fNwkSIntKey,
					},
				},
				phyPayload: lw10PHY,
			},
			{
				name: "mic check LW10 - different session-key",
				storedSessions: []PassiveRoamingDeviceSession{
					{
						SessionID:   id1,
						DevAddr:     lorawan.DevAddr{1, 2, 3, 4},
						LoRaWAN11:   false,
						Lifetime:    time.Now().Add(time.Minute),
						FCntUp:      10,
						ValidateMIC: true,
					},
				},
				phyPayload: lw10PHY,
			},
			{
				name: "mic check LW11",
				storedSessions: []PassiveRoamingDeviceSession{
					{
						SessionID:   id1,
						DevAddr:     lorawan.DevAddr{1, 2, 3, 4},
						Lifetime:    time.Now().Add(time.Minute),
						FCntUp:      10,
						ValidateMIC: true,
						LoRaWAN11:   true,
						FNwkSIntKey: fNwkSIntKey,
					},
				},
				expectedSessions: []PassiveRoamingDeviceSession{
					{
						SessionID:   id1,
						DevAddr:     lorawan.DevAddr{1, 2, 3, 4},
						Lifetime:    time.Now().Add(time.Minute),
						FCntUp:      10,
						ValidateMIC: true,
						LoRaWAN11:   true,
						FNwkSIntKey: fNwkSIntKey,
					},
				},
				phyPayload: lw11PHY,
			},
		}

		for _, tst := range tests {
			t.Run(tst.name, func(t *testing.T) {

				assert := require.New(t)
				assert.NoError(RedisClient().FlushAll().Err())

				for _, s := range tst.storedSessions {
					assert.NoError(SavePassiveRoamingDeviceSession(context.Background(), &s))
				}

				sessions, err := GetPassiveRoamingDeviceSessionsForPHYPayload(context.Background(), tst.phyPayload)
				assert.NoError(err)
				assert.Len(sessions, len(tst.expectedSessions))

				for _, s := range sessions {
					var found bool
					for _, es := range tst.expectedSessions {
						if s.SessionID == es.SessionID {
							found = true
						}
					}

					assert.True(found)
				}
			})
		}
	})
}
