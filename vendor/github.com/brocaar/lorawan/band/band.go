// Package band provides band specific defaults and configuration.
//
// To select the desired band, use the corresponding build tag.
package band

type Modulation string

const (
	LoRaModulation Modulation = "LORA"
	FSKModulation  Modulation = "FSK"
)

// DataRate defines a data rate
type DataRate struct {
	Modulation   Modulation `json:"modulation"`
	SpreadFactor int        `json:"spreadFactor,omitempty"` // used for LoRa
	Bandwidth    int        `json:"bandwidth,omitempty"`    // in kHz, used for LoRa
	DataRate     int        `json:"dataRate,omitempty"`     // bits per second, used for FSK
}

// MaxPayloadSize defines the max payload size
type MaxPayloadSize struct {
	M int // The maximum MACPayload size length
	N int // The maximum application payload length in the absence of the optional FOpt control field
}
