package joinserver

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/brocaar/lorawan"
)

func TestJoinEUIToServer(t *testing.T) {
	assert := require.New(t)

	assert.Equal("https://8.0.7.0.6.0.5.0.4.0.3.0.2.0.1.0.example.com", joinEUIToServer(lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}, ".example.com"))
}
