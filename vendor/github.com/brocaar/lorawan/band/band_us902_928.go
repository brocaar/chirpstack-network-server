package band

import "time"

func newUS902Band() (Band, error) {
	band := Band{
		DefaultTXPower:   20,
		ImplementsCFlist: false,
		RX2Frequency:     923300000,
		RX2DataRate:      8,

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
			{Modulation: LoRaModulation, SpreadFactor: 10, Bandwidth: 125},
			{Modulation: LoRaModulation, SpreadFactor: 9, Bandwidth: 125},
			{Modulation: LoRaModulation, SpreadFactor: 8, Bandwidth: 125},
			{Modulation: LoRaModulation, SpreadFactor: 7, Bandwidth: 125},
			{Modulation: LoRaModulation, SpreadFactor: 8, Bandwidth: 500},
			{}, // RFU
			{}, // RFU
			{}, // RFU
			{Modulation: LoRaModulation, SpreadFactor: 12, Bandwidth: 500},
			{Modulation: LoRaModulation, SpreadFactor: 11, Bandwidth: 500},
			{Modulation: LoRaModulation, SpreadFactor: 10, Bandwidth: 500},
			{Modulation: LoRaModulation, SpreadFactor: 9, Bandwidth: 500},
			{Modulation: LoRaModulation, SpreadFactor: 8, Bandwidth: 500},
			{Modulation: LoRaModulation, SpreadFactor: 7, Bandwidth: 500},
			{}, // RFU
			{}, // RFU
		},

		MaxPayloadSize: []MaxPayloadSize{
			{M: 19, N: 11},
			{M: 61, N: 53},
			{M: 137, N: 129},
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
		},

		RX1DataRate: [][]int{
			{10, 9, 8, 8},
			{11, 10, 9, 8},
			{12, 11, 10, 9},
			{13, 12, 11, 10},
			{13, 13, 12, 11},
			{}, // Not defined
			{}, // Not defined
			{}, // Not defined
			{8, 8, 8, 8},
			{9, 8, 8, 8},
			{10, 9, 8, 8},
			{11, 10, 9, 8},
			{12, 11, 10, 9},
			{13, 12, 11, 10},
		},

		TXPower: []int{
			30,
			28,
			26,
			24,
			22,
			20,
			18,
			16,
			14,
			12,
			10,
			0,
			0,
			0,
			0,
			0,
		},

		UplinkChannels:   make([]Channel, 72),
		DownlinkChannels: make([]Channel, 8),

		getRX1ChannelFunc: func(txChannel int) int {
			return txChannel % 8
		},

		getRX1FrequencyFunc: func(b *Band, txFrequency int) (int, error) {
			uplinkChan, err := b.GetChannel(txFrequency, nil)
			if err != nil {
				return 0, err
			}

			rx1Chan := b.GetRX1Channel(uplinkChan)
			return b.DownlinkChannels[rx1Chan].Frequency, nil
		},
	}

	// initialize uplink channel 0 - 63
	for i := 0; i < 64; i++ {
		band.UplinkChannels[i] = Channel{
			Frequency: 902300000 + (i * 200000),
			DataRates: []int{0, 1, 2, 3},
		}
	}

	// initialize uplink channel 64 - 71
	for i := 0; i < 8; i++ {
		band.UplinkChannels[i+64] = Channel{
			Frequency: 903000000 + (i * 1600000),
			DataRates: []int{4},
		}
	}

	// initialize downlink channel 0 - 7
	for i := 0; i < 8; i++ {
		band.DownlinkChannels[i] = Channel{
			Frequency: 923300000 + (i * 600000),
			DataRates: []int{10, 11, 12, 13},
		}
	}

	return band, nil
}
