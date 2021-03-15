package storage

import (
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/brocaar/chirpstack-network-server/internal/test"
)

type StorageTestSuite struct {
	suite.Suite
	tx *TxLogger
}

func (b *StorageTestSuite) SetupSuite() {
	assert := require.New(b.T())
	conf := test.GetConfig()
	assert.NoError(Setup(conf))
	assert.NoError(MigrateDown(DB().DB))
	assert.NoError(MigrateUp(DB().DB))
}

func (b *StorageTestSuite) SetupTest() {
	tx, err := DB().Beginx()
	if err != nil {
		panic(err)
	}
	b.tx = tx

	if err := MigrateDown(DB().DB); err != nil {
		panic(err)
	}
	if err := MigrateUp(DB().DB); err != nil {
		panic(err)
	}

	RedisClient().FlushAll()
}

func (b *StorageTestSuite) TearDownTest() {
	if err := b.tx.Rollback(); err != nil {
		panic(err)
	}
}

func (b *StorageTestSuite) Tx() sqlx.Ext {
	return b.tx
}

func (b *StorageTestSuite) DB() *DBLogger {
	return DB()
}

func TestStorage(t *testing.T) {
	suite.Run(t, new(StorageTestSuite))
}
