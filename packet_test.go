package loraserver

import (
	"sort"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDataRate(t *testing.T) {
	Convey("Given a DataRate", t, func() {
		dr := DataRate{}
		Convey("Given the LoRa field is set", func() {
			dr.LoRa = "SF12BW125"
			Convey("Then Modulation is LORA", func() {
				So(dr.Modulation(), ShouldEqual, "LORA")
			})
		})
		Convey("Given the FSK field is set", func() {
			dr.FSK = 5000
			Convey("Then Modulation is FSK", func() {
				So(dr.Modulation(), ShouldEqual, "FSK")
			})
		})
	})
}

func TestRXPackets(t *testing.T) {
	Convey("Given a slice of RXPacket with different RSSI values", t, func() {
		rxPackets := RXPackets{
			{RXInfo: RXInfo{RSSI: 1}},
			{RXInfo: RXInfo{RSSI: 3}},
			{RXInfo: RXInfo{RSSI: 7}},
			{RXInfo: RXInfo{RSSI: 9}},
			{RXInfo: RXInfo{RSSI: 6}},
			{RXInfo: RXInfo{RSSI: 4}},
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
