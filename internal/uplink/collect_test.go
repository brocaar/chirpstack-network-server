package uplink

import (
	"fmt"
	"sync"
	"testing"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/test"
	"github.com/brocaar/lorawan"
	. "github.com/smartystreets/goconvey/convey"
)

func TestCollectAndCallOnce(t *testing.T) {
	conf := test.GetTestConfig()

	Convey("Given a Redis connection pool", t, func() {
		p := common.NewRedisPool(conf.RedisURL)
		test.MustFlushRedis(p)

		Convey("Given a single LoRaWAN packet", func() {
			phy := lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MIC:        [4]byte{1, 2, 3, 4},
				MACPayload: &lorawan.MACPayload{},
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

					cb := func(packets models.RXPackets) error {
						called = called + 1
						received = len(packets)
						return nil
					}

					var wg sync.WaitGroup
					for _, g := range test.Gateways {
						wg.Add(1)
						packet := gw.RXPacket{
							RXInfo: gw.RXInfo{
								MAC: g,
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
