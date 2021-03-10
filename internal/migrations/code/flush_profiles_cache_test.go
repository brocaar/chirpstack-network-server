package code

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/brocaar/chirpstack-network-server/internal/test"
)

type FlushProfilesCacheTestSuite struct {
	suite.Suite
}

func (b *FlushProfilesCacheTestSuite) SetupSuite() {
	conf := test.GetConfig()
	if err := storage.Setup(conf); err != nil {
		panic(err)
	}

	test.MustResetDB(storage.DB().DB)
}

func (b *FlushProfilesCacheTestSuite) SetupTest() {
	test.MustResetDB(storage.DB().DB)
}

func (ts *FlushProfilesCacheTestSuite) TestFlushProfilesCache() {
	assert := require.New(ts.T())

	// test a clean database
	assert.NoError(FlushProfilesCache(storage.DB()))

	// create device-profile
	dp := storage.DeviceProfile{}
	assert.NoError(storage.CreateDeviceProfile(context.Background(), storage.DB(), &dp))
	assert.NoError(storage.CreateDeviceProfileCache(context.Background(), dp))
	_, err := storage.GetDeviceProfileCache(context.Background(), dp.ID)
	assert.NoError(err)

	// create service-profile
	sp := storage.ServiceProfile{}
	assert.NoError(storage.CreateServiceProfile(context.Background(), storage.DB(), &sp))
	assert.NoError(storage.CreateServiceProfileCache(context.Background(), sp))
	_, err = storage.GetServiceProfileCache(context.Background(), sp.ID)
	assert.NoError(err)

	// flush cache
	assert.NoError(FlushProfilesCache(storage.DB()))

	// cache should be empty
	_, err = storage.GetDeviceProfileCache(context.Background(), dp.ID)
	assert.Equal(storage.ErrDoesNotExist, err)

	// cache should be empty
	_, err = storage.GetServiceProfileCache(context.Background(), sp.ID)
	assert.Equal(storage.ErrDoesNotExist, err)

	// flush empty cache
	assert.NoError(FlushProfilesCache(storage.DB()))
}

func TestFlushProfilesCache(t *testing.T) {
	suite.Run(t, new(FlushProfilesCacheTestSuite))
}
