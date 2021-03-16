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
	conf := test.GetConfig()
	if err := Setup(conf); err != nil {
		panic(err)
	}

	test.MustResetDB(DB().DB)
}

func (b *StorageTestSuite) SetupTest() {
	tx, err := DB().Beginx()
	if err != nil {
		panic(err)
	}
	b.tx = tx
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
		template string
		params   []interface{}
		expected string
	}{
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
		out := GetRedisKey(tst.template, tst.params...)
		assert.Equal(tst.expected, out)
	}
}
