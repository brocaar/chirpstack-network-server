package testsuite

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type PassiveRoamingFNSTestSuite struct {
	IntegrationTestSuite
}

func (ts *PassiveRoamingFNSTestSuite) TestJoinRequest() {

}

func (ts *PassiveRoamingFNSTestSuite) TestDataStateless() {

}

func (ts *PassiveRoamingFNSTestSuite) TestDataStatefull() {

}

func (ts *PassiveRoamingFNSTestSuite) TestDownlink() {

}

type PassiveRoamingSNSTestSuite struct {
	IntegrationTestSuite
}

// TestPassiveRoamingFNS tests the passive-roaming from the fNS POV.
func TestPassiveRoamingFNS(t *testing.T) {
	suite.Run(t, new(PassiveRoamingFNSTestSuite))
}

// TestPassiveRoamingSNS tests the passive-roaming from the sNS POV.
func TestPassiveRoamingSNS(t *testing.T) {
	suite.Run(t, new(PassiveRoamingSNSTestSuite))
}
