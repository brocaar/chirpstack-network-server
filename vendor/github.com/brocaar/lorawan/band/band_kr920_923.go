package band

import "time"

func newKR920Band() (Band, error) {
	return Band{
		DefaultTXPower:   23, // for gateway
		ImplementsCFlist: true,
		RX2Frequency:     921900000,
		RX2DataRate:      0,

		MaxFCntGap:       16384,
		ADRACKLimit:      64,
		ADRACKDelay:      32,
		ReceiveDelay1:    time.Second,
		ReceiveDelay2:    time.Second * 2,
		JoinAcceptDelay1: time.Second * 5,
		JoinAcceptDelay2: time.Second * 6,
		ACKTimeoutMin:    time.Second,
		ACKTimeoutMax:    time.Second * 3,

		DataRates: []DataRate{
			{Modulation: LoRaModulation, SpreadFactor: 12, Bandwidth: 125},
			{Modulation: LoRaModulation, SpreadFactor: 11, Bandwidth: 125},
			{Modulation: LoRaModulation, SpreadFactor: 10, Bandwidth: 125},
			{Modulation: LoRaModulation, SpreadFactor: 9, Bandwidth: 125},
			{Modulation: LoRaModulation, SpreadFactor: 8, Bandwidth: 125},
			{Modulation: LoRaModulation, SpreadFactor: 7, Bandwidth: 125},
		},

		MaxPayloadSize: []MaxPayloadSize{
			{M: 73, N: 65},
			{M: 159, N: 151},
			{M: 250, N: 242},
			{M: 250, N: 242},
			{M: 250, N: 242},
			{M: 250, N: 242},
		},

		rx1DataRate: [][]int{
			{0, 0, 0, 0, 0, 0},
			{1, 0, 0, 0, 0, 0},
			{2, 1, 0, 0, 0, 0},
			{3, 2, 1, 0, 0, 0},
			{4, 3, 2, 1, 0, 0},
			{5, 4, 3, 2, 1, 0},
		},

		TXPower: []int{
			20,
			14,
			10,
			8,
			5,
			2,
			0,
		},

		UplinkChannels: []Channel{
			{Frequency: 922100000, DataRates: []int{0, 1, 2, 3, 4, 5}},
			{Frequency: 922300000, DataRates: []int{0, 1, 2, 3, 4, 5}},
			{Frequency: 922500000, DataRates: []int{0, 1, 2, 3, 4, 5}},
		},

		DownlinkChannels: []Channel{
			{Frequency: 922100000, DataRates: []int{0, 1, 2, 3, 4, 5}},
			{Frequency: 922300000, DataRates: []int{0, 1, 2, 3, 4, 5}},
			{Frequency: 922500000, DataRates: []int{0, 1, 2, 3, 4, 5}},
		},

		getRX1ChannelFunc: func(txChannel int) int {
			return txChannel
		},

		getRX1FrequencyFunc: func(b *Band, txFrequency int) (int, error) {
			return txFrequency, nil
		},
	}, nil
}
