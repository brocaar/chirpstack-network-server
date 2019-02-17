package maccommand

import (
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/loraserver/internal/test"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type TestBase struct {
	suite.Suite
}

func (ts *TestBase) SetupSuite() {
	assert := require.New(ts.T())
	conf := test.GetConfig()
	assert.NoError(storage.Setup(conf))
}

func (ts *TestBase) SetupTest() {
	test.MustFlushRedis(storage.RedisPool())
}
