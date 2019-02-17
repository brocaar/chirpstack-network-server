package joinserver

import (
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/brocaar/loraserver/internal/test"
	"github.com/brocaar/lorawan"
)

func init() {
	log.SetLevel(log.ErrorLevel)
}

type PoolTestSuite struct {
	suite.Suite
}

func (ts *PoolTestSuite) SetupSuite() {
	assert := require.New(ts.T())

	conf := test.GetConfig()
	conf.JoinServer.ResolveJoinEUI = true
	conf.JoinServer.ResolveDomainSuffix = ".example.com"

	assert.NoError(Setup(conf))
}

func (ts *PoolTestSuite) TestJoinEUIToServer() {
	assert := require.New(ts.T())

	assert.Equal("8.0.7.0.6.0.5.0.4.0.3.0.2.0.1.0.example.com", GetPool().(*pool).joinEUIToServer(lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}))
}

func (ts *PoolTestSuite) TestAToServer() {
	assert := require.New(ts.T())

	tests := []struct {
		Server   string
		Secure   bool
		Port     int
		Expected string
	}{
		{
			Server:   "example.com",
			Secure:   false,
			Port:     80,
			Expected: "http://example.com:80/",
		},
		{
			Server:   "example.com",
			Secure:   true,
			Port:     443,
			Expected: "https://example.com:443/",
		},
	}

	for _, tst := range tests {
		url, err := GetPool().(*pool).aToURL(tst.Server, tst.Secure, tst.Port)
		assert.NoError(err)
		assert.Equal(tst.Expected, url)
	}
}

func TestPool(t *testing.T) {
	suite.Run(t, new(PoolTestSuite))
}
