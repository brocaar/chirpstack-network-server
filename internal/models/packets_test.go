package models

import (
	"sort"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/brocaar/loraserver/api/gw"
)

func TestRXPackets(t *testing.T) {
	Convey("Given a slice of RXPacket with different RSSI values", t, func() {
		rxPackets := RXPackets{
			{RXInfo: gw.RXInfo{RSSI: 1}},
			{RXInfo: gw.RXInfo{RSSI: 3}},
			{RXInfo: gw.RXInfo{RSSI: 7}},
			{RXInfo: gw.RXInfo{RSSI: 9}},
			{RXInfo: gw.RXInfo{RSSI: 6}},
			{RXInfo: gw.RXInfo{RSSI: 4}},
		}

		Convey("After sorting", func() {
			sort.Sort(rxPackets)

			Convey("Then the slice should be sorted, strongest signal first", func() {
				for i, rssi := range []int{9, 7, 6, 4, 3, 1} {
					So(rxPackets[i].RXInfo.RSSI, ShouldEqual, rssi)
				}
			})
		})
	})
}
