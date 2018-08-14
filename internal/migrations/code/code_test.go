package code

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/brocaar/loraserver/internal/test"
)

type MigrateTestSuite struct {
	suite.Suite
	test.DatabaseTestSuiteBase
}

func (ts *MigrateTestSuite) TestMigrate() {
	assert := require.New(ts.T())
	count := 0

	// returning an error does not mark the migration as completed
	assert.Error(Migrate(ts.DB(), "test_1", func() error {
		count++
		return fmt.Errorf("BOOM")
	}))

	assert.Equal(1, count)

	// re-run the migration
	assert.NoError(Migrate(ts.DB(), "test_1", func() error {
		count++
		return nil
	}))

	assert.Equal(2, count)

	// the migration has already been completed
	assert.NoError(Migrate(ts.DB(), "test_1", func() error {
		count++
		return nil
	}))

	assert.Equal(2, count)

	// new migration should run
	assert.NoError(Migrate(ts.DB(), "test_2", func() error {
		count++
		return nil
	}))

	assert.Equal(3, count)

	// migration has already been applied
	assert.NoError(Migrate(ts.DB(), "test_2", func() error {
		count++
		return nil
	}))

	assert.Equal(3, count)
}

func TestMigrate(t *testing.T) {
	suite.Run(t, new(MigrateTestSuite))
}
