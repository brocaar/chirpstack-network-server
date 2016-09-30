package band

import "time"

func newCN470Band() (Band, error) {
	band := Band{
		DefaultTXPower:   14,
		ImplementsCFlist: false,
		RX2Frequency:     505300000,
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
			{}, // RFU
			{}, // RFU
			{}, // RFU
			{}, // RFU
			{}, // RFU
			{}, // RFU
			{}, // RFU
			{}, // RFU
			{}, // RFU
			{}, // RFU
		},

		MaxPayloadSize: []MaxPayloadSize{
			{M: 59, N: 51},
			{M: 59, N: 51},
			{M: 59, N: 51},
			{M: 123, N: 115},
			{M: 230, N: 222},
			{M: 230, N: 222},
			{}, // not defined
			{}, // not defined
			{}, // not defined
			{}, // not defined
			{}, // not defined
			{}, // not defined
			{}, // not defined
			{}, // not defined
			{}, // not defined
			{}, // not defined
		},

		RX1DataRate: [][]int{
			{0, 0, 0, 0, 0, 0},
			{1, 0, 0, 0, 0, 0},
			{2, 1, 0, 0, 0, 0},
			{3, 2, 1, 0, 0, 0},
			{4, 3, 2, 1, 0, 0},
			{5, 4, 3, 2, 1, 0},
		},

		TXPower: []int{
			17,
			16,
			14,
			12,
			10,
			7,
			5,
			2,
			0, // rfu
			0, // rfu
			0, // rfu
			0, // rfu
			0, // rfu
			0, // rfu
			0, // rfu
			0, // rfu
		},

		UplinkChannels:   make([]Channel, 96),
		DownlinkChannels: make([]Channel, 48),

		getRX1ChannelFunc: func(txChannel int) int {
			return txChannel % 48
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

	// initialize uplink channels
	for i := 0; i < 96; i++ {
		band.UplinkChannels[i] = Channel{
			Frequency: 470300000 + (i * 200000),
			DataRates: []int{0, 1, 2, 3, 4, 5},
		}
	}

	// initialize downlink channels
	for i := 0; i < 48; i++ {
		band.DownlinkChannels[i] = Channel{
			Frequency: 500300000 + (i * 200000),
			DataRates: []int{0, 1, 2, 3, 4, 5},
		}
	}

	return band, nil
}
