package band

import (
	"fmt"
	"time"

	"github.com/brocaar/lorawan"
)

func newAS923Band(repeaterCompatible bool, dt lorawan.DwellTime) (Band, error) {
	var maxPayloadSize []MaxPayloadSize

	if dt == lorawan.DwellTime400ms {
		if repeaterCompatible {
			maxPayloadSize = []MaxPayloadSize{
				{M: 0, N: 0},
				{M: 0, N: 0},
				{M: 19, N: 11},
				{M: 61, N: 53},
				{M: 134, N: 126},
				{M: 250, N: 242},
				{M: 250, N: 242},
				{M: 250, N: 242},
			}
		} else {
			maxPayloadSize = []MaxPayloadSize{
				{M: 0, N: 0},
				{M: 0, N: 0},
				{M: 19, N: 11},
				{M: 61, N: 53},
				{M: 134, N: 126},
				{M: 250, N: 242},
				{M: 250, N: 242},
				{M: 250, N: 242},
			}
		}
	} else {
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
	}

	return Band{
		dwellTime: dt,

		DefaultTXPower:   14,
		ImplementsCFlist: true,
		RX2Frequency:     923200000,
		RX2DataRate:      2,

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

		TXPower: []int{
			14,
			14 - 2,
			14 - 4,
			14 - 6,
			14 - 8,
			14 - 10,
		},

		UplinkChannels: []Channel{
			{Frequency: 923200000, DataRates: []int{0, 1, 2, 3, 4, 5}},
			{Frequency: 923400000, DataRates: []int{0, 1, 2, 3, 4, 5}},
		},

		DownlinkChannels: []Channel{
			{Frequency: 923200000, DataRates: []int{0, 1, 2, 3, 4, 5}},
			{Frequency: 923400000, DataRates: []int{0, 1, 2, 3, 4, 5}},
		},

		getRX1DataRateFunc: func(band *Band, uplinkDR, rx1DROffset int) (int, error) {
			if rx1DROffset < 0 || rx1DROffset > 7 {
				return 0, fmt.Errorf("lorawan/band: invalid RX1 data-rate offset: %d", rx1DROffset)
			}

			if uplinkDR < 0 || uplinkDR > 7 {
				return 0, fmt.Errorf("lorawan/band: invalid uplink data-rate: %d", uplinkDR)
			}

			minDR := 0
			if band.dwellTime == lorawan.DwellTime400ms {
				minDR = 2
			}

			effectiveRX1DROffset := []int{0, 1, 2, 3, 4, 5, -1, -2}[rx1DROffset]
			dr := uplinkDR - effectiveRX1DROffset

			if dr < minDR {
				dr = minDR
			}

			if dr > 5 {
				dr = 5
			}

			return dr, nil
		},

		getRX1ChannelFunc: func(txChannel int) int {
			return txChannel
		},

		getRX1FrequencyFunc: func(b *Band, txFrequency int) (int, error) {
			return txFrequency, nil
		},
	}, nil
}
