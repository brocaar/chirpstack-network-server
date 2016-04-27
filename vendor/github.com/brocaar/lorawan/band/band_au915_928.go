// +build au_915_928

package band

import (
	"fmt"
	"time"
)

func init() {
	// initialize uplink channel 0 - 63
	for i := 0; i < 64; i++ {
		UplinkChannelConfiguration[i] = Channel{
			Frequency: 915200000 + (i * 200000),
			DataRates: []int{0, 1, 2, 3},
		}
	}

	// initialize uplink channel 64 - 71
	for i := 0; i < 8; i++ {
		UplinkChannelConfiguration[i+64] = Channel{
			Frequency: 915900000 + (i * 1600000),
			DataRates: []int{4},
		}
	}

	// initialize downlink channel 0 - 7
	for i := 0; i < 8; i++ {
		DownlinkChannelConfiguration[i] = Channel{
			Frequency: 923300000 + (i * 600000),
			DataRates: []int{8, 9, 10, 11, 12, 13},
		}
	}
}

// Name defines the name of the band
const Name = "AU 915-928"

// DataRateConfiguration defines the available data rates
var DataRateConfiguration = [...]DataRate{
	{Modulation: LoRaModulation, SpreadFactor: 10, Bandwidth: 125}, //0
	{Modulation: LoRaModulation, SpreadFactor: 9, Bandwidth: 125},  //1
	{Modulation: LoRaModulation, SpreadFactor: 8, Bandwidth: 125},  //2
	{Modulation: LoRaModulation, SpreadFactor: 7, Bandwidth: 125},  //3
	{Modulation: LoRaModulation, SpreadFactor: 8, Bandwidth: 500},  //4
	{}, // RFU
	{}, // RFU
	{}, // RFU
	{Modulation: LoRaModulation, SpreadFactor: 12, Bandwidth: 500}, //8
	{Modulation: LoRaModulation, SpreadFactor: 11, Bandwidth: 500}, //9
	{Modulation: LoRaModulation, SpreadFactor: 10, Bandwidth: 500}, //10
	{Modulation: LoRaModulation, SpreadFactor: 9, Bandwidth: 500},  //11
	{Modulation: LoRaModulation, SpreadFactor: 8, Bandwidth: 500},  //12
	{Modulation: LoRaModulation, SpreadFactor: 7, Bandwidth: 500},  //13
	{}, // RFU
	{}, // RFU
}

// DefaultTXPower defines the default TX power in dBm
const DefaultTXPower = 20

// CFListAllowed defines if the optional JoinAccept CFList is allowed for this band
const CFListAllowed = false

// LoRaWAN 1.0. Standard - Section 7.5.3
// TXPowerConfiguration defines the available TXPower settings in dBm
var TXPowerConfiguration = [...]int{
	30, // 0
	28, // 1
	26, // 2
	24, // 3
	22, // 4
	20, // 5
	18, // 6
	16, // 7
	14, // 8
	12, // 9
	10, // 10
	0,  // RFU
	0,
	0,
	0,
	0,
}

// MACPayloadSizeConfiguration defines the maximum payload size for each data rate
var MACPayloadSizeConfiguration = [...]MaxPayloadSize{
	{M: 19, N: 11},
	{M: 61, N: 53},
	{M: 134, N: 126},
	{M: 250, N: 242},
	{M: 250, N: 242},
	{}, // Not defined
	{}, // Not defined
	{}, // Not defined
	{M: 41, N: 33},
	{M: 117, N: 109},
	{M: 230, N: 222},
	{M: 230, N: 222},
	{M: 230, N: 222},
	{M: 230, N: 222},
	{}, // Not defined
	{}, // Not defined
}

// RX1DROffsetConfiguration defines the available RX1DROffset configurations
// per data rate.
var RX1DROffsetConfiguration = [...][4]int{
	{10, 9, 8, 8},
	{11, 10, 9, 8},
	{12, 11, 10, 9},
	{13, 12, 11, 10},
	{13, 13, 12, 11},
}

// RX2Frequency defines the RX2 receive window frequency to use (in Hz)
const RX2Frequency = 923300000

// RX2DataRate defines the RX2 receive window data rate to use
const RX2DataRate = 8

// UplinkChannelConfiguration defines the (default) available uplink channels.
var UplinkChannelConfiguration = [72]Channel{}

// DownlinkChannelConfiguration defines the (default) available downlink channels.
var DownlinkChannelConfiguration = [8]Channel{}

// GetRX1Frequency returns the frequency to be used for RX1 given
// the uplink frequency and data rate.
func GetRX1Frequency(frequency, dataRate int) (int, error) {
	if dataRate > len(DataRateConfiguration) {
		return 0, fmt.Errorf("lorawan/band: given data rate: %d does not exist", dataRate)
	}

	chanNum, err := getChannelNumber(frequency, dataRate)
	if err != nil {
		return 0, err
	}

	return DownlinkChannelConfiguration[chanNum%8].Frequency, nil
}

// To get a channel number from a frequency in MHz
func getChannelNumber(frequency, dataRate int) (int, error) {
	for chanNum, channel := range UplinkChannelConfiguration {
		if frequency == channel.Frequency {
			for _, dr := range channel.DataRates {
				if dr == dataRate {
					return chanNum, nil
				}
			}
		}
	}

	return 0, fmt.Errorf("lorawan/band: could not get channel number for frequency: %d, data rate: %d", frequency, dataRate)
}

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
