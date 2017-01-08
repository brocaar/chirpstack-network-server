// Package band provides band specific defaults and configuration for
// downlink communication with end-nodes.
package band

import (
	"errors"
	"fmt"
	"time"

	"github.com/brocaar/lorawan"
)

// Name defines the band-name type.
type Name string

// Available ISM bands.
const (
	AS_923     Name = "AS_923"
	AU_915_928 Name = "AU_915_928"
	CN_470_510 Name = "CN_470_510"
	CN_779_787 Name = "CN_779_787"
	EU_433     Name = "EU_433"
	EU_863_870 Name = "EU_863_870"
	KR_920_923 Name = "KR_920_923"
	RU_864_869 Name = "RU_864_869"
	US_902_928 Name = "US_902_928"
)

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

// Band defines an region specific ISM band implementation for LoRa.
type Band struct {
	// dwellTime defines if dwell time limitation should be taken into account
	dwellTime lorawan.DwellTime

	// rx1DataRate defines the RX1 data-rate given the uplink data-rate
	// and a RX1DROffset value. These values are retrievable by using
	// the GetRX1DataRate method.
	rx1DataRate [][]int

	// DefaultTXPower defines the default radiated transmit output power
	DefaultTXPower int

	// ImplementsCFlist defines if the band implements the optional channel
	// frequency list.
	ImplementsCFlist bool

	// RX2Frequency defines the fixed frequency for the RX2 receive window
	RX2Frequency int

	// RX2DataRate defines the fixed data-rate for the RX2 receive window
	RX2DataRate int

	// MaxFcntGap defines the MAC_FCNT_GAP default value.
	MaxFCntGap uint32

	// ADRACKLimit defines the ADR_ACK_LIMIT default value.
	ADRACKLimit int

	// ADRACKDelay defines the ADR_ACK_DELAY default value.
	ADRACKDelay int

	// ReceiveDelay1 defines the RECEIVE_DELAY1 default value.
	ReceiveDelay1 time.Duration

	// ReceiveDelay2 defines the RECEIVE_DELAY2 default value.
	ReceiveDelay2 time.Duration

	// JoinAcceptDelay1 defines the JOIN_ACCEPT_DELAY1 default value.
	JoinAcceptDelay1 time.Duration

	// JoinAcceptDelay2 defines the JOIN_ACCEPT_DELAY2 default value.
	JoinAcceptDelay2 time.Duration

	// ACKTimeoutMin defines the ACK_TIMEOUT min. default value.
	ACKTimeoutMin time.Duration

	// ACKTimeoutMax defines the ACK_TIMEOUT max. default value.
	ACKTimeoutMax time.Duration

	// DataRates defines the available data rates.
	DataRates []DataRate

	// MaxPayloadSize defines the maximum payload size, per data-rate.
	MaxPayloadSize []MaxPayloadSize

	// TXPower defines the TX power configuration.
	TXPower []int

	// UplinkChannels defines the list of (default) configured uplink channels.
	UplinkChannels []Channel

	// DownlinkChannels defines the list of (default) configured downlink
	// channels.
	DownlinkChannels []Channel

	// getRX1ChannelFunc implements a function which returns the RX1 channel
	// based on the uplink / TX channel.
	getRX1ChannelFunc func(txChannel int) int

	// getRX1FrequencyFunc implements a function which returns the RX1 frequency
	// given the uplink frequency.
	getRX1FrequencyFunc func(band *Band, txFrequency int) (int, error)

	// getRX1DataRateFunc implements a function which returns the RX1 data-rate
	// given the uplink data-rate and data-rate offset.
	getRX1DataRateFunc func(band *Band, uplinkDR, rx1DROffset int) (int, error)
}

// GetRX1Channel returns the channel to use for RX1 given the channel used
// for uplink.
func (b *Band) GetRX1Channel(txChannel int) int {
	return b.getRX1ChannelFunc(txChannel)
}

// GetRX1Frequency returns the frequency to use for RX1 given the uplink
// frequency.
func (b *Band) GetRX1Frequency(txFrequency int) (int, error) {
	return b.getRX1FrequencyFunc(b, txFrequency)
}

// GetRX1DataRate returns the RX1 data-rate given the uplink data-rate and
// RX1 data-rate offset.
func (b *Band) GetRX1DataRate(uplinkDR, rx1DROffset int) (int, error) {
	// use the lookup table when no function has been defined
	if b.getRX1DataRateFunc == nil {
		return b.rx1DataRate[uplinkDR][rx1DROffset], nil
	}
	return b.getRX1DataRateFunc(b, uplinkDR, rx1DROffset)
}

// GetChannel returns the channel index given a frequency and an optional CFList.
func (b *Band) GetChannel(frequency int, cFlist *lorawan.CFList) (int, error) {
	for chanNum, channel := range b.UplinkChannels {
		if frequency == channel.Frequency {
			return chanNum, nil
		}
	}

	if cFlist != nil {
		for chanNum, channel := range cFlist {
			if frequency == int(channel) {
				return chanNum + len(b.UplinkChannels), nil
			}
		}
	}

	return 0, fmt.Errorf("lorawan/band: unknown channel for frequency: %d", frequency)
}

// GetDataRate returns the index of the given DataRate.
func (b *Band) GetDataRate(dr DataRate) (int, error) {
	for i, d := range b.DataRates {
		if d == dr {
			return i, nil
		}
	}
	return 0, errors.New("lorawan/band: the given data-rate does not exist")
}

// GetRX1DataRateForOffset returns the data-rate for the given offset.
func (b *Band) GetRX1DataRateForOffset(dr, drOffset int) (int, error) {
	if dr >= len(b.rx1DataRate) {
		return 0, fmt.Errorf("lorawan/band: invalid data-rate: %d", dr)
	}

	if drOffset >= len(b.rx1DataRate[dr]) {
		return 0, fmt.Errorf("lorawan/band: invalid data-rate offset: %d", drOffset)
	}
	return b.rx1DataRate[dr][drOffset], nil
}

// GetConfig returns the band configuration for the given band.
// Please refer to the LoRaWAN specification for more details about the effect
// of the repeater and dwell time arguments.
func GetConfig(name Name, repeaterCompatible bool, dt lorawan.DwellTime) (Band, error) {
	switch name {
	case AS_923:
		return newAS923Band(repeaterCompatible, dt)
	case AU_915_928:
		return newAU915Band(repeaterCompatible)
	case CN_470_510:
		return newCN470Band()
	case CN_779_787:
		return newCN779Band(repeaterCompatible)
	case EU_433:
		return newEU433Band(repeaterCompatible)
	case EU_863_870:
		return newEU863Band(repeaterCompatible)
	case KR_920_923:
		return newKR920Band()
	case RU_864_869:
		return newRU864Band(repeaterCompatible)
	case US_902_928:
		return newUS902Band(repeaterCompatible)
	default:
		return Band{}, fmt.Errorf("lorawan/band: band %s is undefined", name)
	}
}
