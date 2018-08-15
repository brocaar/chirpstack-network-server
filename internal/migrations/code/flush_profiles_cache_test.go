package code

import (
	"testing"

	"github.com/brocaar/loraserver/internal/storage"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/brocaar/loraserver/internal/test"
)

type FlushProfilesCacheTestSuite struct {
	suite.Suite
	test.DatabaseTestSuiteBase
}

func (ts *FlushProfilesCacheTestSuite) TestFlushProfilesCache() {
	assert := require.New(ts.T())

	// test a clean database
	assert.NoError(FlushProfilesCache(ts.RedisPool(), ts.DB()))

	// create device-profile
	dp := storage.DeviceProfile{}
	assert.NoError(storage.CreateDeviceProfile(ts.DB(), &dp))
	assert.NoError(storage.CreateDeviceProfileCache(ts.RedisPool(), dp))
	_, err := storage.GetDeviceProfileCache(ts.RedisPool(), dp.ID)
	assert.NoError(err)

	// create service-profile
	sp := storage.ServiceProfile{}
	assert.NoError(storage.CreateServiceProfile(ts.DB(), &sp))
	assert.NoError(storage.CreateServiceProfileCache(ts.RedisPool(), sp))
	_, err = storage.GetServiceProfileCache(ts.RedisPool(), sp.ID)
	assert.NoError(err)

	// flush cache
	assert.NoError(FlushProfilesCache(ts.RedisPool(), ts.DB()))

	// cache should be empty
	_, err = storage.GetDeviceProfileCache(ts.RedisPool(), dp.ID)
	assert.Equal(storage.ErrDoesNotExist, err)

	// cache should be empty
	_, err = storage.GetServiceProfileCache(ts.RedisPool(), sp.ID)
	assert.Equal(storage.ErrDoesNotExist, err)

	// flush empty cache
	assert.NoError(FlushProfilesCache(ts.RedisPool(), ts.DB()))
}

func TestFlushProfilesCache(t *testing.T) {
	suite.Run(t, new(FlushProfilesCacheTestSuite))
}
