package roaming

import (
	"testing"
	"time"

	"github.com/brocaar/chirpstack-network-server/internal/config"
	"github.com/brocaar/chirpstack-network-server/internal/test"
	"github.com/brocaar/lorawan"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestIsRoamingDevAddr(t *testing.T) {
	netID = lorawan.NetID{1, 2, 3}

	t.Run("DevAddr is roaming", func(t *testing.T) {
		assert := require.New(t)
		roamingEnabled = true

		devAddr := lorawan.DevAddr{6, 7, 8, 9}
		devAddr.SetAddrPrefix(lorawan.NetID{3, 2, 1})

		assert.True(IsRoamingDevAddr(devAddr))
	})

	t.Run("DevAddr is not roaming", func(t *testing.T) {
		assert := require.New(t)
		roamingEnabled = true

		devAddr := lorawan.DevAddr{6, 7, 8, 9}
		devAddr.SetAddrPrefix(netID)

		assert.False(IsRoamingDevAddr(devAddr))
	})

	t.Run("DevAddr is roaming, roaming disabled", func(t *testing.T) {
		assert := require.New(t)
		roamingEnabled = false

		devAddr := lorawan.DevAddr{6, 7, 8, 9}
		devAddr.SetAddrPrefix(lorawan.NetID{3, 2, 1})

		assert.False(IsRoamingDevAddr(devAddr))
	})
}

func TestGetClientForNetID(t *testing.T) {
	assert := require.New(t)

	conf := test.GetConfig()
	conf.Roaming.Servers = []config.RoamingServer{
		{
			NetID:          lorawan.NetID{6, 6, 6},
			PassiveRoaming: true,
		},
	}
	assert.NoError(Setup(conf))

	t.Run("Roaming agreement", func(t *testing.T) {
		assert := require.New(t)
		c, err := GetClientForNetID(lorawan.NetID{6, 6, 6})
		assert.NoError(err)
		assert.NotNil(c)
	})

	t.Run("No roaming agreement", func(t *testing.T) {
		assert := require.New(t)
		_, err := GetClientForNetID(lorawan.NetID{6, 6, 7})
		assert.Equal(ErrNoAgreement, errors.Cause(err))
	})

	t.Run("No roaming agreement, default client enabled", func(t *testing.T) {
		assert := require.New(t)
		conf.Roaming.Default.Enabled = true
		conf.Roaming.Default.PassiveRoaming = true
		assert.NoError(Setup(conf))

		c, err := GetClientForNetID(lorawan.NetID{6, 6, 7})
		assert.NoError(err)
		assert.NotNil(c)
	})
}

func TestGetPassiveRoamingLifetime(t *testing.T) {
	assert := require.New(t)

	conf := test.GetConfig()
	conf.Roaming.Servers = []config.RoamingServer{
		{
			NetID:                  lorawan.NetID{6, 6, 6},
			PassiveRoaming:         true,
			PassiveRoamingLifetime: time.Hour,
		},
	}
	assert.NoError(Setup(conf))

	t.Run("Roaming agreement", func(t *testing.T) {
		assert := require.New(t)
		assert.Equal(time.Hour, GetPassiveRoamingLifetime(lorawan.NetID{6, 6, 6}))
	})

	t.Run("No roaming agreement", func(t *testing.T) {
		assert := require.New(t)
		assert.Equal(time.Duration(0), GetPassiveRoamingLifetime(lorawan.NetID{6, 6, 7}))
	})

	t.Run("Default roaming agreement", func(t *testing.T) {
		assert := require.New(t)

		conf := test.GetConfig()
		conf.Roaming.Servers = []config.RoamingServer{
			{
				NetID:                  lorawan.NetID{6, 6, 6},
				PassiveRoaming:         true,
				PassiveRoamingLifetime: time.Hour,
			},
		}
		conf.Roaming.Default.Enabled = true
		conf.Roaming.Default.PassiveRoamingLifetime = time.Minute
		assert.NoError(Setup(conf))

		assert.Equal(time.Minute, GetPassiveRoamingLifetime(lorawan.NetID{6, 6, 7}))
	})
}

func TestGetPassiveRoamingKEKLabel(t *testing.T) {
	assert := require.New(t)
	conf := test.GetConfig()
	conf.Roaming.Servers = []config.RoamingServer{
		{
			NetID:                  lorawan.NetID{6, 6, 6},
			PassiveRoaming:         true,
			PassiveRoamingKEKLabel: "test-kek",
		},
	}
	assert.NoError(Setup(conf))

	t.Run("Roaming agreement", func(t *testing.T) {
		assert := require.New(t)
		assert.Equal("test-kek", GetPassiveRoamingKEKLabel(lorawan.NetID{6, 6, 6}))
	})

	t.Run("No roaming agreement", func(t *testing.T) {
		assert := require.New(t)
		assert.Equal("", GetPassiveRoamingKEKLabel(lorawan.NetID{6, 6, 7}))
	})

	t.Run("Default roaming agreement", func(t *testing.T) {
		assert := require.New(t)
		conf := test.GetConfig()
		conf.Roaming.Servers = []config.RoamingServer{
			{
				NetID:                  lorawan.NetID{6, 6, 6},
				PassiveRoaming:         true,
				PassiveRoamingKEKLabel: "test-kek",
			},
		}
		conf.Roaming.Default.Enabled = true
		conf.Roaming.Default.PassiveRoamingKEKLabel = "default-kek"
		assert.NoError(Setup(conf))

		assert.Equal("default-kek", GetPassiveRoamingKEKLabel(lorawan.NetID{6, 6, 7}))
	})
}

func TestGetNetIDsForDevAddr(t *testing.T) {
	assert := require.New(t)
	conf := test.GetConfig()
	conf.Roaming.Servers = []config.RoamingServer{
		{
			NetID:                  lorawan.NetID{6, 6, 6},
			PassiveRoaming:         true,
			PassiveRoamingKEKLabel: "test-kek",
		},
	}
	assert.NoError(Setup(conf))

	t.Run("Roaming agreement", func(t *testing.T) {
		assert := require.New(t)

		devAddr := lorawan.DevAddr{6, 7, 8, 9}
		devAddr.SetAddrPrefix(lorawan.NetID{6, 6, 6})

		assert.Equal([]lorawan.NetID{{6, 6, 6}}, GetNetIDsForDevAddr(devAddr))
	})

	t.Run("No roaming agreement", func(t *testing.T) {
		assert := require.New(t)

		devAddr := lorawan.DevAddr{6, 7, 8, 9}
		devAddr.SetAddrPrefix(lorawan.NetID{6, 6, 7})

		assert.Len(GetNetIDsForDevAddr(devAddr), 0)
	})
}
