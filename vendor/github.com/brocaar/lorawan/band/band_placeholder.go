// +build !eu_863_870

package band

import "time"

// Name defines the name of the band
const Name = "placeholder"

// DataRateConfiguration defines the available data rates
var DataRateConfiguration = [0]DataRate{}

// DefaultTXPower defines the default TX power in dBm
const DefaultTXPower = 0

// CFListAllowed defines if the optional JoinAccept CFList is allowed for this band
const CFListAllowed = false

// TXPowerConfiguration defines the available TXPower settings in dBm
var TXPowerConfiguration = [0]int{}

// MACPayloadSizeConfiguration defines the maximum payload size for each data rate
var MACPayloadSizeConfiguration = [0]MaxPayloadSize{}

// RX1DROffsetConfiguration defines the available RX1DROffset configurations
// per data rate.
var RX1DROffsetConfiguration = [0][6]int{}

// RX2Frequency defines the RX2 receive window frequency to use (in Hz)
const RX2Frequency = 0

// RX2DataRate defines the RX2 receive window data rate to use
const RX2DataRate = 0

// Default settings for this band
const (
	ReceiveDelay1    time.Duration = 0
	ReceiveDelay2    time.Duration = 0
	JoinAcceptDelay1 time.Duration = 0
	JoinAcceptDelay2 time.Duration = 0
	MaxFCntGap       uint32        = 0
	ADRAckLimit                    = 0
	ADRAckDelay                    = 0
	AckTimeoutMin    time.Duration = 0
	AckTimeoutMax    time.Duration = 0
)
