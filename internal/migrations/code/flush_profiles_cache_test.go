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
	test.MustFlushRedis(storage.RedisPool())
}

func (ts *FlushProfilesCacheTestSuite) TestFlushProfilesCache() {
	assert := require.New(ts.T())

	// test a clean database
	assert.NoError(FlushProfilesCache(storage.RedisPool(), storage.DB()))

	// create device-profile
	dp := storage.DeviceProfile{}
	assert.NoError(storage.CreateDeviceProfile(context.Background(), storage.DB(), &dp))
	assert.NoError(storage.CreateDeviceProfileCache(context.Background(), storage.RedisPool(), dp))
	_, err := storage.GetDeviceProfileCache(context.Background(), storage.RedisPool(), dp.ID)
	assert.NoError(err)

	// create service-profile
	sp := storage.ServiceProfile{}
	assert.NoError(storage.CreateServiceProfile(context.Background(), storage.DB(), &sp))
	assert.NoError(storage.CreateServiceProfileCache(context.Background(), storage.RedisPool(), sp))
	_, err = storage.GetServiceProfileCache(context.Background(), storage.RedisPool(), sp.ID)
	assert.NoError(err)

	// flush cache
	assert.NoError(FlushProfilesCache(storage.RedisPool(), storage.DB()))

	// cache should be empty
	_, err = storage.GetDeviceProfileCache(context.Background(), storage.RedisPool(), dp.ID)
	assert.Equal(storage.ErrDoesNotExist, err)

	// cache should be empty
	_, err = storage.GetServiceProfileCache(context.Background(), storage.RedisPool(), sp.ID)
	assert.Equal(storage.ErrDoesNotExist, err)

	// flush empty cache
	assert.NoError(FlushProfilesCache(storage.RedisPool(), storage.DB()))
}

func TestFlushProfilesCache(t *testing.T) {
	suite.Run(t, new(FlushProfilesCacheTestSuite))
}
