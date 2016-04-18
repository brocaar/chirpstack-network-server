// Package band provides band specific defaults and configuration.
//
// To select the desired band, use the corresponding build tag.
package band

import "errors"

// Modulation defines the modulation type.
type Modulation string

// Possible modulation types.
const (
	LoRaModulation Modulation = "LORA"
	FSKModulation  Modulation = "FSK"
)

// DataRate defines a data rate
type DataRate struct {
	Modulation   Modulation `json:"modulation"`
	SpreadFactor int        `json:"spreadFactor,omitempty"` // used for LoRa
	Bandwidth    int        `json:"bandwidth,omitempty"`    // in kHz, used for LoRa
	BitRate      int        `json:"bitRate,omitempty"`      // bits per second, used for FSK
}

// MaxPayloadSize defines the max payload size
type MaxPayloadSize struct {
	M int // The maximum MACPayload size length
	N int // The maximum application payload length in the absence of the optional FOpt control field
}

// Channel defines the channel structure
type Channel struct {
	Frequency int   // frequency in Hz
	DataRates []int // each int mapping to an index in DataRateConfiguration
}

// GetDataRate returns the index of the given DataRate.
func GetDataRate(dr DataRate) (int, error) {
	for i, d := range DataRateConfiguration {
		if d == dr {
			return i, nil
		}
	}
	return 0, errors.New("lorawan/band: the given DataRate does not exist")
}
