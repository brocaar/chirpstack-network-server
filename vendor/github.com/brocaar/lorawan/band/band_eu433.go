package band

import "time"

func newEU433Band(repeaterCompatible bool) (Band, error) {
	var maxPayloadSize []MaxPayloadSize

	if repeaterCompatible {
		maxPayloadSize = []MaxPayloadSize{
			{M: 59, N: 51},
			{M: 59, N: 51},
			{M: 59, N: 51},
			{M: 123, N: 115},
			{M: 230, N: 222},
			{M: 230, N: 222},
			{M: 230, N: 222},
			{M: 230, N: 222},
		}
	} else {
		maxPayloadSize = []MaxPayloadSize{
			{M: 59, N: 51},
			{M: 59, N: 51},
			{M: 59, N: 51},
			{M: 123, N: 115},
			{M: 250, N: 242},
			{M: 250, N: 242},
			{M: 250, N: 242},
			{M: 250, N: 242},
		}
	}

	return Band{
		DefaultTXPower:   10,
		ImplementsCFlist: true,
		RX2Frequency:     434665000,
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
			{Modulation: LoRaModulation, SpreadFactor: 7, Bandwidth: 250},
			{Modulation: FSKModulation, BitRate: 50000},
		},

		MaxPayloadSize: maxPayloadSize,

		rx1DataRate: [][]int{
			{0, 0, 0, 0, 0, 0},
			{1, 0, 0, 0, 0, 0},
			{2, 1, 0, 0, 0, 0},
			{3, 2, 1, 0, 0, 0},
			{4, 3, 2, 1, 0, 0},
			{5, 4, 3, 2, 1, 0},
			{6, 5, 4, 3, 2, 1},
			{7, 6, 5, 4, 3, 2},
		},

		TXPowerOffset: []int{
			0,
			-2,
			-4,
			-6,
			-8,
			-10,
		},

		UplinkChannels: []Channel{
			{Frequency: 433175000, DataRates: []int{0, 1, 2, 3, 4, 5}},
			{Frequency: 433375000, DataRates: []int{0, 1, 2, 3, 4, 5}},
			{Frequency: 433575000, DataRates: []int{0, 1, 2, 3, 4, 5}},
		},

		DownlinkChannels: []Channel{
			{Frequency: 433175000, DataRates: []int{0, 1, 2, 3, 4, 5}},
			{Frequency: 433375000, DataRates: []int{0, 1, 2, 3, 4, 5}},
			{Frequency: 433575000, DataRates: []int{0, 1, 2, 3, 4, 5}},
		},

		getRX1ChannelFunc: func(txChannel int) int {
			return txChannel
		},

		getRX1FrequencyFunc: func(b *Band, txFrequency int) (int, error) {
			return txFrequency, nil
		},
	}, nil
}
