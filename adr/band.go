package adr

import (
	"github.com/brocaar/chirpstack-network-server/v3/internal/band"
	loraband "github.com/brocaar/lorawan/band"
)

// Band returns the configured band.
func Band() loraband.Band {
	return band.Band()
}