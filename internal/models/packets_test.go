package models

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/brocaar/loraserver/api/gw"
)

func TestRXInfoSet(t *testing.T) {
	assert := require.New(t)

	rxInfoSet := []*gw.UplinkRXInfo{
		{LoraSnr: 4, Rssi: 1},
		{LoraSnr: 0, Rssi: 10},
		{LoraSnr: 0, Rssi: 30},
		{LoraSnr: 0, Rssi: 20},
		{LoraSnr: 3, Rssi: 1},
		{LoraSnr: 3, Rssi: 3},
		{LoraSnr: 3, Rssi: 2},
		{LoraSnr: 6, Rssi: 5},
		{LoraSnr: 7, Rssi: 15},
		{LoraSnr: 8, Rssi: 10},
	}

	sort.Sort(BySignalStrength(rxInfoSet))

	rxInfoSetExpected := []*gw.UplinkRXInfo{
		{LoraSnr: 7, Rssi: 15},
		{LoraSnr: 8, Rssi: 10},
		{LoraSnr: 6, Rssi: 5},
		{LoraSnr: 4, Rssi: 1},
		{LoraSnr: 3, Rssi: 3},
		{LoraSnr: 3, Rssi: 2},
		{LoraSnr: 3, Rssi: 1},
		{LoraSnr: 0, Rssi: 30},
		{LoraSnr: 0, Rssi: 20},
		{LoraSnr: 0, Rssi: 10},
	}

	assert.Equal(rxInfoSetExpected, rxInfoSet)
}
