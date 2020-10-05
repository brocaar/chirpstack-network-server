package joinserver

import (
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"github.com/brocaar/chirpstack-network-server/internal/test"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/backend"
)

func TestJoinEUIToServer(t *testing.T) {
	assert := require.New(t)

	assert.Equal("https://8.0.7.0.6.0.5.0.4.0.3.0.2.0.1.0.example.com", joinEUIToServer(lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}, ".example.com"))
}

func TestGetClientForJoinEUI(t *testing.T) {
	assert := require.New(t)
	conf := test.GetConfig()
	assert.NoError(Setup(conf))

	t.Run("Pre-configured client", func(t *testing.T) {
		assert := require.New(t)

		joinEUI := lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}

		client, err := backend.NewClient(backend.ClientConfig{
			Logger: log.StandardLogger(),
		})
		assert.NoError(err)

		servers = append(servers, serverItem{
			joinEUI: joinEUI,
			client:  client,
		})

		cl, err := GetClientForJoinEUI(joinEUI)
		assert.NoError(err)
		assert.EqualValues(client, cl)
	})

	t.Run("Default client", func(t *testing.T) {
		assert := require.New(t)

		joinEUI := lorawan.EUI64{2, 2, 3, 4, 5, 6, 7, 8}

		client, err := GetClientForJoinEUI(joinEUI)
		assert.NoError(err)

		defClient, err := getDefaultClient(joinEUI)
		assert.NoError(err)
		assert.EqualValues(defClient, client)
	})
}
