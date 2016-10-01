package models

import (
	"sort"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRXInfoSet(t *testing.T) {
	Convey("Given an unsorted RXInfoSet slice", t, func() {
		rxInfoSet := RXInfoSet{
			{LoRaSNR: 4, RSSI: 1},
			{LoRaSNR: 0, RSSI: 10},
			{LoRaSNR: 0, RSSI: 30},
			{LoRaSNR: 0, RSSI: 20},
			{LoRaSNR: 3, RSSI: 1},
			{LoRaSNR: 3, RSSI: 3},
			{LoRaSNR: 3, RSSI: 2},
			{LoRaSNR: 6, RSSI: 5},
			{LoRaSNR: 7, RSSI: 15},
			{LoRaSNR: 8, RSSI: 10},
		}

		Convey("After sorting", func() {
			sort.Sort(rxInfoSet)

			Convey("Then the RXInfoSet is in the expected order", func() {
				So(rxInfoSet, ShouldResemble, RXInfoSet{
					{LoRaSNR: 7, RSSI: 15},
					{LoRaSNR: 8, RSSI: 10},
					{LoRaSNR: 6, RSSI: 5},
					{LoRaSNR: 4, RSSI: 1},
					{LoRaSNR: 3, RSSI: 3},
					{LoRaSNR: 3, RSSI: 2},
					{LoRaSNR: 3, RSSI: 1},
					{LoRaSNR: 0, RSSI: 30},
					{LoRaSNR: 0, RSSI: 20},
					{LoRaSNR: 0, RSSI: 10},
				})
			})
		})
	})
}
