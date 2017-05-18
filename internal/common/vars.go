package common

import (
	"time"

	"github.com/brocaar/lorawan/band"
)

// NodeSessionTTL defines the TTL of a node session (will be renewed on each
// activity)
var NodeSessionTTL = time.Hour * 24 * 31

// Band is the ISM band configuration to use
var Band band.Band

// BandName is the name of the used ISM band
var BandName band.Name

// DeduplicationDelay holds the time to wait for uplink de-duplication
var DeduplicationDelay = time.Millisecond * 200

// GetDownlinkDataDelay holds the delay between uplink delivery to the app server and getting the downlink data from the app server (if any)
var GetDownlinkDataDelay = time.Millisecond * 100

// TimeLocation holds the timezone location
var TimeLocation = time.Local

// CreateGatewayOnStats defines if non-existing gateways should be created
// automatically when receiving stats.
var CreateGatewayOnStats = false

// SpreadFactorToRequiredSNRTable contains the required SNR to demodulate a
// LoRa frame for the given spreadfactor.
// These values are taken from the SX1276 datasheet.
var SpreadFactorToRequiredSNRTable = map[int]float64{
	6:  -5,
	7:  -7.5,
	8:  -10,
	9:  -12.5,
	10: -15,
	11: -17.5,
	12: -20,
}
