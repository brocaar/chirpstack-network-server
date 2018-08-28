package storage

import (
	"testing"

	"github.com/brocaar/loraserver/internal/test"
	"github.com/stretchr/testify/suite"
)

type StorageTestSuite struct {
	suite.Suite
	test.DatabaseTestSuiteBase
}

func TestStorage(t *testing.T) {
	suite.Run(t, new(StorageTestSuite))
}
