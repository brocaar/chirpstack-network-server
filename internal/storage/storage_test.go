package storage

import (
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/brocaar/chirpstack-network-server/v3/internal/test"
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

func TestGetRedisKey(t *testing.T) {
	assert := require.New(t)

	tests := []struct {
		keyPrefix string
		template  string
		params    []interface{}
		expected  string
	}{
		{
			keyPrefix: "eu868:",
			template:  "foo:bar:key",
			expected:  "eu868:foo:bar:key",
		},
		{
			template: "foo:bar:key",
			expected: "foo:bar:key",
		},
		{
			template: "foo:bar:%s",
			params:   []interface{}{"test"},
			expected: "foo:bar:test",
		},
	}

	for _, tst := range tests {
		keyPrefix = tst.keyPrefix
		out := GetRedisKey(tst.template, tst.params...)
		assert.Equal(tst.expected, out)
	}
}
