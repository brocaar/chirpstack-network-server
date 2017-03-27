package gateway

import (
	"testing"
	"time"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/test"
	"github.com/brocaar/lorawan"
	"github.com/pkg/errors"
	. "github.com/smartystreets/goconvey/convey"
)

func TestGatewayStatsAggregation(t *testing.T) {
	conf := test.GetConfig()
	db, err := common.OpenDatabase(conf.PostgresDSN)
	if err != nil {
		t.Fatal(err)
	}

	Convey("Given a clean database", t, func() {
		common.CreateGatewayOnStats = false
		So(err, ShouldBeNil)
		test.MustResetDB(db)

		MustSetStatsAggregationIntervals([]string{"SECOND", "MINUTE"})
		lat := float64(1.123)
		long := float64(1.124)
		alt := float64(15.3)

		stats := gw.GatewayStatsPacket{
			MAC:                 [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
			Latitude:            &lat,
			Longitude:           &long,
			Altitude:            &alt,
			RXPacketsReceived:   11,
			RXPacketsReceivedOK: 9,
			TXPacketsReceived:   13,
			TXPacketsEmitted:    10,
		}

		Convey("When CreateGatewayOnStats=false", func() {
			Convey("Then an error is returned on stats", func() {
				err := handleStatsPacket(db, stats)
				So(err, ShouldNotBeNil)
				So(errors.Cause(err), ShouldResemble, ErrDoesNotExist)
			})
		})

		Convey("When CreateGatewayOnStats=true", func() {
			common.CreateGatewayOnStats = true

			Convey("Then the gateway is created automatically on stats", func() {
				So(handleStatsPacket(db, stats), ShouldBeNil)

				gw, err := GetGateway(db, stats.MAC)
				So(err, ShouldBeNil)
				So(gw.CreatedAt, ShouldNotBeNil)
				So(gw.UpdatedAt, ShouldNotBeNil)
				So(gw.FirstSeenAt, ShouldNotBeNil)
				So(gw.FirstSeenAt, ShouldResemble, gw.LastSeenAt)
				So(gw.Location, ShouldResemble, &GPSPoint{
					Latitude:  lat,
					Longitude: long,
				})
				So(gw.Altitude, ShouldResemble, &alt)
			})
		})

		Convey("Given a gateway in the database", func() {
			gw := Gateway{
				MAC:  [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
				Name: "test-gateway",
			}
			So(CreateGateway(db, &gw), ShouldBeNil)

			Convey("When aggregating 3 stats over an interval of a second", func() {
				start := time.Now().Truncate(time.Hour).In(time.UTC)
				for i := 0; i < 3; i++ {
					stats.Time = start.Add(time.Duration(i) * time.Second)
					So(handleStatsPacket(db, stats), ShouldBeNil)
				}

				Convey("Then the stats can be retrieved on second level", func() {
					stats, err := GetGatewayStats(db, gw.MAC, "SECOND", start, start.Add(3*time.Second))
					So(err, ShouldBeNil)
					So(stats, ShouldHaveLength, 4)
					for i := range stats {
						stats[i].Timestamp = stats[i].Timestamp.In(time.UTC)
					}
					So(stats, ShouldResemble, []Stats{
						{MAC: gw.MAC, Timestamp: start, Interval: "SECOND", RXPacketsReceived: 11, RXPacketsReceivedOK: 9, TXPacketsReceived: 13, TXPacketsEmitted: 10},
						{MAC: gw.MAC, Timestamp: start.Add(time.Second), Interval: "SECOND", RXPacketsReceived: 11, RXPacketsReceivedOK: 9, TXPacketsReceived: 13, TXPacketsEmitted: 10},
						{MAC: gw.MAC, Timestamp: start.Add(2 * time.Second), Interval: "SECOND", RXPacketsReceived: 11, RXPacketsReceivedOK: 9, TXPacketsReceived: 13, TXPacketsEmitted: 10},
						{MAC: gw.MAC, Timestamp: start.Add(3 * time.Second), Interval: "SECOND", RXPacketsReceived: 0, RXPacketsReceivedOK: 0, TXPacketsReceived: 0, TXPacketsEmitted: 0},
					})
				})

				Convey("Then the stats can be retrieved on a minute level", func() {
					stats, err := GetGatewayStats(db, gw.MAC, "MINUTE", start, start.Add(3*time.Second))
					So(err, ShouldBeNil)
					So(stats, ShouldHaveLength, 1)
					for i := range stats {
						stats[i].Timestamp = stats[i].Timestamp.In(time.UTC)
					}
					So(stats, ShouldResemble, []Stats{
						{MAC: gw.MAC, Timestamp: start, Interval: "MINUTE", RXPacketsReceived: 33, RXPacketsReceivedOK: 27, TXPacketsReceived: 39, TXPacketsEmitted: 30},
					})

					Convey("Then the start timestamp is truncated to minute precision", func() {
						// so that when requestion 23:55:44, minute 55 is still included
						stats, err := GetGatewayStats(db, gw.MAC, "MINUTE", start.Add(3*time.Second), start.Add((3 * time.Second)))
						So(err, ShouldBeNil)
						So(stats, ShouldHaveLength, 1)
					})
				})
			})
		})
	})
}

func TestGatewayFunctions(t *testing.T) {
	conf := test.GetConfig()

	Convey("Given a clean database", t, func() {
		db, err := common.OpenDatabase(conf.PostgresDSN)
		So(err, ShouldBeNil)
		test.MustResetDB(db)

		Convey("When creating a gateway", func() {
			gw := Gateway{
				Name: "test-gateway",
				MAC:  lorawan.EUI64{29, 238, 8, 208, 182, 145, 209, 73},
				Location: &GPSPoint{
					Latitude:  1.23456789,
					Longitude: 4.56789012,
				},
			}
			So(CreateGateway(db, &gw), ShouldBeNil)

			// some precicion will get lost when writing to the db
			// truncate it to ms precision for comparison
			gw.CreatedAt = gw.CreatedAt.UTC().Truncate(time.Millisecond)
			gw.UpdatedAt = gw.UpdatedAt.UTC().Truncate(time.Millisecond)

			Convey("Then it can be retrieved", func() {
				gw2, err := GetGateway(db, gw.MAC)
				So(err, ShouldBeNil)

				gw2.CreatedAt = gw2.CreatedAt.UTC().Truncate(time.Millisecond)
				gw2.UpdatedAt = gw2.UpdatedAt.UTC().Truncate(time.Millisecond)

				So(gw2, ShouldResemble, gw)

				gws, err := GetGatewaysForMACs(db, []lorawan.EUI64{gw.MAC})
				So(err, ShouldBeNil)
				gw3, ok := gws[gw.MAC]
				So(ok, ShouldBeTrue)
				So(gw3.MAC, ShouldResemble, gw.MAC)
			})

			Convey("Then it can be updated", func() {
				now := time.Now().UTC().Truncate(time.Millisecond)
				altitude := 100.5

				gw.FirstSeenAt = &now
				gw.LastSeenAt = &now
				gw.Location.Latitude = 1.23456780
				gw.Location.Longitude = 5.56789012
				gw.Altitude = &altitude

				So(UpdateGateway(db, &gw), ShouldBeNil)

				gw2, err := GetGateway(db, gw.MAC)
				So(err, ShouldBeNil)

				So(gw2.MAC, ShouldEqual, gw.MAC)
				So(gw2.CreatedAt.UTC().Truncate(time.Millisecond), ShouldResemble, gw.CreatedAt.UTC())
				So(gw2.UpdatedAt.UTC().Truncate(time.Millisecond), ShouldResemble, gw.UpdatedAt.UTC().Truncate(time.Millisecond))
				So(gw2.FirstSeenAt.UTC().Truncate(time.Millisecond), ShouldResemble, gw.FirstSeenAt.UTC().Truncate(time.Millisecond))
				So(gw2.LastSeenAt.UTC().Truncate(time.Millisecond), ShouldResemble, gw.LastSeenAt.UTC().Truncate(time.Millisecond))
				So(gw2.Location, ShouldResemble, gw.Location)
				So(gw2.Altitude, ShouldResemble, gw.Altitude)
			})

			Convey("Then the gateway count is 1", func() {
				count, err := GetGatewayCount(db)
				So(err, ShouldBeNil)
				So(count, ShouldEqual, 1)
			})

			Convey("Then listing the gateways returns the expected item", func() {
				gws, err := GetGateways(db, 10, 0)
				So(err, ShouldBeNil)
				So(gws, ShouldHaveLength, 1)

				gws[0].CreatedAt = gws[0].CreatedAt.UTC().Truncate(time.Millisecond)
				gws[0].UpdatedAt = gws[0].UpdatedAt.UTC().Truncate(time.Millisecond)
				So(gws[0], ShouldResemble, gw)
			})

			Convey("Then it can be deleted", func() {
				So(DeleteGateway(db, gw.MAC), ShouldBeNil)
				_, err := GetGateway(db, gw.MAC)
				So(err, ShouldResemble, ErrDoesNotExist)
			})
		})
	})
}
