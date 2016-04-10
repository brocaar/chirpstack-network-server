// +build eu_863_870

package band

import "time"

// Name defines the name of the band
const Name = "EU 863-870"

// DataRateConfiguration defines the available data rates
var DataRateConfiguration = [...]DataRate{
	{Modulation: LoRaModulation, SpreadFactor: 12, Bandwith: 125},
	{Modulation: LoRaModulation, SpreadFactor: 11, Bandwith: 125},
	{Modulation: LoRaModulation, SpreadFactor: 10, Bandwith: 125},
	{Modulation: LoRaModulation, SpreadFactor: 9, Bandwith: 125},
	{Modulation: LoRaModulation, SpreadFactor: 8, Bandwith: 125},
	{Modulation: LoRaModulation, SpreadFactor: 7, Bandwith: 125},
	{Modulation: LoRaModulation, SpreadFactor: 7, Bandwith: 250},
	{Modulation: FSKModulation, DataRate: 50000},
}

// DefaultTXPower defines the default TX power in dBm
const DefaultTXPower = 14

// CFListAllowed defines if the optional JoinAccept CFList is allowed for this band
const CFListAllowed = true

// TXPowerConfiguration defines the available TXPower settings in dBm
var TXPowerConfiguration = [...]int{
	20, // if supported
	14,
	11,
	8,
	5,
	2,
}

// MACPayloadSizeConfiguration defines the maximum payload size for each data rate
var MACPayloadSizeConfiguration = [...]MaxPayloadSize{
	{M: 59, N: 51},
	{M: 59, N: 51},
	{M: 59, N: 51},
	{M: 123, N: 115},
	{M: 230, N: 222},
	{M: 230, N: 222},
	{M: 230, N: 222},
	{M: 230, N: 222},
}

// RX1DROffsetConfiguration defines the available RX1DROffset configurations
// per data rate.
var RX1DROffsetConfiguration = [...][6]int{
	{0, 0, 0, 0, 0, 0},
	{1, 0, 0, 0, 0, 0},
	{2, 1, 0, 0, 0, 0},
	{3, 2, 1, 0, 0, 0},
	{4, 3, 2, 1, 0, 0},
	{5, 4, 3, 2, 1, 0},
	{6, 5, 4, 3, 2, 1},
	{7, 6, 5, 4, 3, 2},
}

// RX2Frequency defines the RX2 receive window frequency to use (in Hz)
const RX2Frequency = 869525000

// RX2DataRate defines the RX2 receive window data rate to use
const RX2DataRate = 0

// Default settings for this band
const (
	ReceiveDelay1    time.Duration = time.Second
	ReceiveDelay2    time.Duration = time.Second * 2
	JoinAcceptDelay1 time.Duration = time.Second * 5
	JoinAcceptDelay2 time.Duration = time.Second * 6
	MaxFCntGap       uint32        = 16384
	ADRAckLimit                    = 64
	ADRAckDelay                    = 32
	AckTimeoutMin    time.Duration = time.Second // AckTimeout = 2 +/- 1 (random value between 1 - 3)
	AckTimeoutMax    time.Duration = time.Second * 3
)
