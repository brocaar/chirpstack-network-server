package adr

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestADR(t *testing.T) {
	t.Run("GetHandler", func(t *testing.T) {
		assert := require.New(t)
		h := GetHandler("default")
		id, err := h.ID()
		assert.NoError(err)
		assert.Equal("default", id)
	})
}
