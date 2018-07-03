package storage

import (
	"testing"
	"time"

	"github.com/brocaar/lorawan/band"

	"github.com/brocaar/lorawan"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/test"
	"github.com/pkg/errors"
	. "github.com/smartystreets/goconvey/convey"
)

func TestGatewayStatsAggregation(t *testing.T) {
	conf := test.GetConfig()
	db, err := common.OpenDatabase(conf.PostgresDSN)
	if err != nil {
		t.Fatal(err)
	}
	config.C.PostgreSQL.DB = db

	Convey("Given a clean database", t, func() {
		config.C.NetworkServer.Gateway.Stats.CreateGatewayOnStats = false
		So(err, ShouldBeNil)
		test.MustResetDB(config.C.PostgreSQL.DB)

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
				err := HandleGatewayStatsPacket(db, stats)
				So(err, ShouldNotBeNil)
				So(errors.Cause(err), ShouldResemble, ErrDoesNotExist)
			})
		})

		Convey("When CreateGatewayOnStats=true", func() {
			config.C.NetworkServer.Gateway.Stats.CreateGatewayOnStats = true

			Convey("Then the gateway is created automatically on stats", func() {
				So(HandleGatewayStatsPacket(db, stats), ShouldBeNil)

				gw, err := GetGateway(db, stats.MAC)
				So(err, ShouldBeNil)
				So(gw.CreatedAt, ShouldNotBeNil)
				So(gw.UpdatedAt, ShouldNotBeNil)
				So(gw.FirstSeenAt, ShouldNotBeNil)
				So(gw.FirstSeenAt, ShouldResemble, gw.LastSeenAt)
				So(gw.Location, ShouldResemble, GPSPoint{
					Latitude:  lat,
					Longitude: long,
				})
				So(gw.Altitude, ShouldResemble, alt)
			})
		})

		Convey("Given a gateway in the database", func() {
			gw := Gateway{
				MAC: [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
			}
			So(CreateGateway(db, &gw), ShouldBeNil)

			Convey("When aggregating 3 stats over an interval of a second", func() {
				start := time.Now().Truncate(time.Hour).In(time.UTC)
				for i := 0; i < 3; i++ {
					stats.Time = start.Add(time.Duration(i) * time.Second)
					So(HandleGatewayStatsPacket(db, stats), ShouldBeNil)
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

func TestHandleConfigurationUpdate(t *testing.T) {
	conf := test.GetConfig()
	db, err := common.OpenDatabase(conf.PostgresDSN)
	if err != nil {
		t.Fatal(err)
	}

	gwBackend := test.NewGatewayBackend()

	config.C.PostgreSQL.DB = db
	config.C.NetworkServer.Gateway.Backend.Backend = gwBackend

	Convey("Given a clean database with a gateway", t, func() {
		test.MustResetDB(db)

		g := Gateway{
			MAC: lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
		}
		So(CreateGateway(db, &g), ShouldBeNil)

		Convey("When calling handleConfigurationUpdate", func() {
			So(handleConfigurationUpdate(db, g, ""), ShouldBeNil)

			Convey("Then no gateway-configuration was updated", func() {
				So(gwBackend.GatewayConfigPacketChan, ShouldHaveLength, 0)
			})
		})

		Convey("Given the gateway has a gateway-profile", func() {
			var err error
			gp := GatewayProfile{
				Channels: []int64{0, 1, 2},
				ExtraChannels: []ExtraChannel{
					{
						Modulation:       string(band.LoRaModulation),
						Frequency:        867100000,
						Bandwidth:        125,
						SpreadingFactors: []int64{7, 8, 9, 10, 11, 12},
					},
					{
						Modulation: string(band.FSKModulation),
						Frequency:  868800000,
						Bandwidth:  125,
						Bitrate:    50000,
					},
				},
			}
			So(CreateGatewayProfile(db, &gp), ShouldBeNil)
			gp, err = GetGatewayProfile(db, gp.ID)
			So(err, ShouldBeNil)

			g.GatewayProfileID = &gp.ID
			So(UpdateGateway(db, &g), ShouldBeNil)

			Convey("When calling handleConfigurationUpdate", func() {
				So(handleConfigurationUpdate(db, g, ""), ShouldBeNil)

				Convey("Then the gateway-configuration was published", func() {
					So(gwBackend.GatewayConfigPacketChan, ShouldHaveLength, 1)
					So(<-gwBackend.GatewayConfigPacketChan, ShouldResemble, gw.GatewayConfigPacket{
						Version: gp.GetVersion(),
						MAC:     g.MAC,
						Channels: []gw.Channel{
							{
								Modulation:       band.LoRaModulation,
								Frequency:        868100000,
								Bandwidth:        125,
								SpreadingFactors: []int{7, 8, 9, 10, 11, 12},
							},
							{
								Modulation:       band.LoRaModulation,
								Frequency:        868300000,
								Bandwidth:        125,
								SpreadingFactors: []int{7, 8, 9, 10, 11, 12},
							},
							{
								Modulation:       band.LoRaModulation,
								Frequency:        868500000,
								Bandwidth:        125,
								SpreadingFactors: []int{7, 8, 9, 10, 11, 12},
							},
							{
								Modulation:       band.LoRaModulation,
								Frequency:        867100000,
								Bandwidth:        125,
								SpreadingFactors: []int{7, 8, 9, 10, 11, 12},
							},
							{
								Modulation: band.FSKModulation,
								Frequency:  868800000,
								Bandwidth:  125,
								Bitrate:    50000,
							},
						},
					})
				})
			})
		})
	})
}
