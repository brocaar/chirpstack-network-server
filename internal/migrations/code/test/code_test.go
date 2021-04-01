package test

import (
	"fmt"
	"testing"

	"github.com/brocaar/chirpstack-network-server/v3/internal/migrations/code"
	"github.com/brocaar/chirpstack-network-server/v3/internal/storage"
	"github.com/brocaar/chirpstack-network-server/v3/internal/test"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type MigrateTestSuite struct {
	suite.Suite
}

func (b *MigrateTestSuite) SetupSuite() {
	assert := require.New(b.T())
	conf := test.GetConfig()
	if err := storage.Setup(conf); err != nil {
		panic(err)
	}
	assert.NoError(storage.Setup(conf))
	assert.NoError(storage.MigrateDown(storage.DB().DB))
	assert.NoError(storage.MigrateUp(storage.DB().DB))
}

func (ts *MigrateTestSuite) TestMigrate() {
	assert := require.New(ts.T())
	count := 0

	// returning an error does not mark the migration as completed
	assert.Error(code.Migrate(storage.DB().DB, "test_1", func(db sqlx.Ext) error {
		count++
		return fmt.Errorf("BOOM")
	}))

	assert.Equal(1, count)

	// re-run the migration
	assert.NoError(code.Migrate(storage.DB().DB, "test_1", func(db sqlx.Ext) error {
		count++
		return nil
	}))

	assert.Equal(2, count)

	// the migration has already been completed
	assert.NoError(code.Migrate(storage.DB().DB, "test_1", func(db sqlx.Ext) error {
		count++
		return nil
	}))

	assert.Equal(2, count)

	// new migration should run
	assert.NoError(code.Migrate(storage.DB().DB, "test_2", func(db sqlx.Ext) error {
		count++
		return nil
	}))

	assert.Equal(3, count)

	// migration has already been applied
	assert.NoError(code.Migrate(storage.DB().DB, "test_2", func(db sqlx.Ext) error {
		count++
		return nil
	}))

	assert.Equal(3, count)
}

func TestMigrate(t *testing.T) {
	suite.Run(t, new(MigrateTestSuite))
}
