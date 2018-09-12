package storage

import (
	"testing"
	"time"

	"github.com/brocaar/lorawan"
	"github.com/stretchr/testify/require"
)

func (ts *StorageTestSuite) TestDevice() {
	assert := require.New(ts.T())

	sp := ServiceProfile{}
	dp := DeviceProfile{}
	rp := RoutingProfile{}

	assert.Nil(CreateServiceProfile(ts.Tx(), &sp))
	assert.Nil(CreateDeviceProfile(ts.Tx(), &dp))
	assert.Nil(CreateRoutingProfile(ts.Tx(), &rp))

	ts.T().Run("Create", func(t *testing.T) {
		assert := require.New(t)

		d := Device{
			DevEUI:            lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
			ServiceProfileID:  sp.ID,
			DeviceProfileID:   dp.ID,
			RoutingProfileID:  rp.ID,
			SkipFCntCheck:     true,
			ReferenceAltitude: 5.6,
		}

		assert.Nil(CreateDevice(ts.Tx(), &d))

		d.CreatedAt = d.CreatedAt.Round(time.Second).UTC()
		d.UpdatedAt = d.UpdatedAt.Round(time.Second).UTC()

		t.Run("Get", func(t *testing.T) {
			dGet, err := GetDevice(ts.Tx(), d.DevEUI)
			assert.Nil(err)

			dGet.CreatedAt = dGet.CreatedAt.Round(time.Second).UTC()
			dGet.UpdatedAt = dGet.UpdatedAt.Round(time.Second).UTC()

			assert.Equal(d, dGet)
		})

		t.Run("Update", func(t *testing.T) {
			assert := require.New(t)

			spNew := ServiceProfile{}
			dpNew := DeviceProfile{}
			rpNew := RoutingProfile{}

			assert.Nil(CreateServiceProfile(ts.Tx(), &spNew))
			assert.Nil(CreateDeviceProfile(ts.Tx(), &dpNew))
			assert.Nil(CreateRoutingProfile(ts.Tx(), &rpNew))

			d.ServiceProfileID = spNew.ID
			d.DeviceProfileID = dpNew.ID
			d.RoutingProfileID = rpNew.ID
			d.SkipFCntCheck = false
			d.ReferenceAltitude = 6.7

			assert.Nil(UpdateDevice(ts.Tx(), &d))
			d.UpdatedAt = d.UpdatedAt.Round(time.Second).UTC()

			dGet, err := GetDevice(ts.Tx(), d.DevEUI)
			assert.Nil(err)

			dGet.CreatedAt = dGet.CreatedAt.Round(time.Second).UTC()
			dGet.UpdatedAt = dGet.UpdatedAt.Round(time.Second).UTC()

			assert.Equal(d, dGet)
		})

		t.Run("Delete", func(t *testing.T) {
			assert := require.New(t)

			assert.Nil(DeleteDevice(ts.Tx(), d.DevEUI))
			assert.Equal(ErrDoesNotExist, DeleteDevice(ts.Tx(), d.DevEUI))
			_, err := GetDevice(ts.Tx(), d.DevEUI)
			assert.Equal(ErrDoesNotExist, err)
		})
	})
}

func (ts *StorageTestSuite) TestDeviceActivation() {
	assert := require.New(ts.T())

	sp := ServiceProfile{}
	dp := DeviceProfile{}
	rp := RoutingProfile{}

	assert.Nil(CreateServiceProfile(ts.Tx(), &sp))
	assert.Nil(CreateDeviceProfile(ts.Tx(), &dp))
	assert.Nil(CreateRoutingProfile(ts.Tx(), &rp))

	d := Device{
		DevEUI:           lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
		ServiceProfileID: sp.ID,
		DeviceProfileID:  dp.ID,
		RoutingProfileID: rp.ID,
		SkipFCntCheck:    true,
	}
	assert.Nil(CreateDevice(ts.Tx(), &d))

	ts.T().Run("Create", func(t *testing.T) {
		assert := require.New(t)

		joinEUI := lorawan.EUI64{1, 2, 1, 2, 1, 2, 1, 2}
		da := DeviceActivation{
			DevEUI:      d.DevEUI,
			JoinEUI:     joinEUI,
			DevAddr:     lorawan.DevAddr{1, 2, 3, 4},
			SNwkSIntKey: lorawan.AES128Key{1, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8},
			FNwkSIntKey: lorawan.AES128Key{2, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8},
			NwkSEncKey:  lorawan.AES128Key{3, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8},
			DevNonce:    258,
			JoinReqType: lorawan.JoinRequestType,
		}

		assert.Nil(CreateDeviceActivation(ts.Tx(), &da))

		t.Run("GetLastDeviceActivationForDevEUI", func(t *testing.T) {
			assert := require.New(t)

			da2 := DeviceActivation{
				DevEUI:      d.DevEUI,
				JoinEUI:     joinEUI,
				DevAddr:     lorawan.DevAddr{4, 3, 2, 1},
				SNwkSIntKey: lorawan.AES128Key{8, 7, 6, 5, 4, 3, 2, 1, 8, 7, 6, 5, 4, 3, 2, 1},
				FNwkSIntKey: lorawan.AES128Key{8, 7, 6, 5, 4, 3, 2, 1, 8, 7, 6, 5, 4, 3, 2, 2},
				NwkSEncKey:  lorawan.AES128Key{8, 7, 6, 5, 4, 3, 2, 1, 8, 7, 6, 5, 4, 3, 2, 3},
				DevNonce:    513,
				JoinReqType: lorawan.JoinRequestType,
			}
			assert.Nil(CreateDeviceActivation(ts.Tx(), &da2))
			da2.CreatedAt = da2.CreatedAt.Round(time.Second).UTC()

			daGet, err := GetLastDeviceActivationForDevEUI(ts.Tx(), d.DevEUI)
			assert.Nil(err)
			daGet.CreatedAt = daGet.CreatedAt.Round(time.Second).UTC()

			assert.Equal(da2, daGet)
		})

		t.Run("ValidateDevNonce for used dev-nonce errors", func(t *testing.T) {
			assert := require.New(t)
			assert.Equal(ErrAlreadyExists, ValidateDevNonce(ts.Tx(), joinEUI, d.DevEUI, da.DevNonce, lorawan.JoinRequestType))
		})

		t.Run("ValidateDevNonce for unused dev-nonce does not error", func(t *testing.T) {
			assert := require.New(t)
			assert.Equal(ErrAlreadyExists, ValidateDevNonce(ts.Tx(), joinEUI, d.DevEUI, lorawan.DevNonce(513), lorawan.JoinRequestType))
		})
	})

}
