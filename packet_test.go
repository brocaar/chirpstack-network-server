package loraserver

import (
	"fmt"
	"sort"
	"sync"
	"testing"

	"github.com/brocaar/lorawan"
	. "github.com/smartystreets/goconvey/convey"
)

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

func TestCollectAndCallOnce(t *testing.T) {
	conf := getConfig()

	Convey("Given a Redis connection pool", t, func() {
		p := NewRedisPool(conf.RedisURL)
		c := p.Get()
		_, err := c.Do("FLUSHALL")
		So(err, ShouldBeNil)
		c.Close()

		Convey("Given a single LoRaWAN packet", func() {
			phy := lorawan.NewPHYPayload(true)
			phy.MIC = [4]byte{1, 2, 3, 4}
			phy.MACPayload = &lorawan.MACPayload{}
			phy.MHDR = lorawan.MHDR{
				MType: lorawan.UnconfirmedDataUp,
				Major: lorawan.LoRaWANR1,
			}

			testTable := []struct {
				Gateways []lorawan.EUI64
				Count    int
			}{
				{
					[]lorawan.EUI64{
						{1, 1, 1, 1, 1, 1, 1, 1},
					},
					1,
				}, {
					[]lorawan.EUI64{
						{1, 1, 1, 1, 1, 1, 1, 1},
						{2, 2, 2, 2, 2, 2, 2, 2},
					},
					2,
				}, {
					[]lorawan.EUI64{
						{1, 1, 1, 1, 1, 1, 1, 1},
						{2, 2, 2, 2, 2, 2, 2, 2},
						{2, 2, 2, 2, 2, 2, 2, 2},
					},
					2,
				},
			}

			for i, test := range testTable {
				Convey(fmt.Sprintf("When running test %d, then %d packets are expected", i, test.Count), func() {
					var received int
					var called int

					cb := func(packets RXPackets) error {
						called = called + 1
						received = len(packets)
						return nil
					}

					var wg sync.WaitGroup
					for _, gw := range test.Gateways {
						wg.Add(1)
						packet := RXPacket{
							RXInfo: RXInfo{
								MAC: gw,
							},
							PHYPayload: phy,
						}
						go func() {
							err := collectAndCallOnce(p, packet, cb)
							if err != nil {
								t.Error(err)
							}
							wg.Done()
						}()
					}
					wg.Wait()

					So(called, ShouldEqual, 1)
					So(received, ShouldEqual, test.Count)
				})
			}
		})

	})
}
