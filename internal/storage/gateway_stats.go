package storage

import (
	"fmt"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	commonPB "github.com/brocaar/loraserver/api/common"
	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/helpers"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/band"
)

// statsAggregationIntervals contains a slice of aggregation intervals.
var statsAggregationIntervals []string

// MustSetStatsAggregationIntervals sets the aggregation intervals to use.
// Valid levels are: SECOND, MINUTE, HOUR, DAY, WEEK, MONTH, QUARTER, YEAR.
func MustSetStatsAggregationIntervals(levels []string) {
	valid := []string{
		"SECOND",
		"MINUTE",
		"HOUR",
		"DAY",
		"WEEK",
		"MONTH",
		"QUARTER",
		"YEAR",
	}
	statsAggregationIntervals = []string{}

	for _, level := range levels {
		found := false
		lUpper := strings.ToUpper(level)
		for _, v := range valid {
			if lUpper == v {
				statsAggregationIntervals = append(statsAggregationIntervals, lUpper)
				found = true
			}
		}
		if !found {
			log.Fatalf("'%s' is not a valid aggregation level", level)
		}
	}
}

// Stats represents a single gateway stats record.
type Stats struct {
	GatewayID           lorawan.EUI64 `db:"gateway_id"`
	Timestamp           time.Time     `db:"timestamp"`
	Interval            string        `db:"interval"`
	RXPacketsReceived   int           `db:"rx_packets_received"`
	RXPacketsReceivedOK int           `db:"rx_packets_received_ok"`
	TXPacketsReceived   int           `db:"tx_packets_received"`
	TXPacketsEmitted    int           `db:"tx_packets_emitted"`
}

// GetGatewayStats returns the stats for the given gateway.
// Note that the stats will return a record for each interval.
func GetGatewayStats(db *common.DBLogger, gatewayID lorawan.EUI64, interval string, start, end time.Time) ([]Stats, error) {
	var valid bool
	interval = strings.ToUpper(interval)

	// validate aggregation interval
	for _, i := range statsAggregationIntervals {
		if i == interval {
			valid = true
		}
	}
	if !valid {
		return nil, ErrInvalidAggregationInterval
	}

	tx, err := db.Beginx()
	if err != nil {
		return nil, errors.Wrap(err, "begin transaction error")
	}
	defer tx.Rollback()

	// set the database timezone for this transaction
	if config.C.NetworkServer.Gateway.Stats.TimezoneLocation != time.Local {
		// when TimeLocation == time.Local, it would have 'Local' as name
		_, err = tx.Exec(fmt.Sprintf("set local time zone '%s'", config.C.NetworkServer.Gateway.Stats.TimezoneLocation.String()))
		if err != nil {
			return nil, errors.Wrap(err, "set timezone error")
		}
	}

	var stats []Stats
	err = tx.Select(&stats, `
		select
			$1::bytea as gateway_id,
			$2 as interval,
			s.timestamp,
			$2 as "interval",
			coalesce(gs.rx_packets_received, 0) as rx_packets_received,
			coalesce(gs.rx_packets_received_ok, 0) as rx_packets_received_ok,
			coalesce(gs.tx_packets_received, 0) as tx_packets_received,
			coalesce(gs.tx_packets_emitted, 0) as tx_packets_emitted
		from (
			select
				*
			from gateway_stats
			where
				gateway_id = $1
				and interval = $2
				and "timestamp" >= cast(date_trunc($2, $3::timestamptz) as timestamp with time zone)
				and "timestamp" < $4) gs
		right join (
			select generate_series(
				cast(date_trunc($2, $3) as timestamp with time zone),
				$4,
				$5) as "timestamp"
			) s
			on gs.timestamp = s.timestamp
		order by s.timestamp`,
		gatewayID[:],
		interval,
		start,
		end,
		fmt.Sprintf("1 %s", interval),
	)
	if err != nil {
		return nil, handlePSQLError(err, "select error")
	}
	return stats, nil
}

// HandleGatewayStatsPacket handles a received stats packet by the gateway.
func HandleGatewayStatsPacket(db *common.DBLogger, stats gw.GatewayStats) error {
	var location GPSPoint
	var altitude float64
	gatewayID := helpers.GetGatewayID(&stats)

	if stats.Location != nil {
		location = GPSPoint{
			Latitude:  stats.Location.Latitude,
			Longitude: stats.Location.Longitude,
		}
		altitude = stats.Location.Altitude
	}

	// create or update the gateway
	gw, err := GetGateway(db, gatewayID)
	if err != nil {
		if err == ErrDoesNotExist && config.C.NetworkServer.Gateway.Stats.CreateGatewayOnStats {
			// create the gateway
			now := time.Now()

			gw = Gateway{
				GatewayID:   gatewayID,
				FirstSeenAt: &now,
				LastSeenAt:  &now,
				Location:    location,
				Altitude:    altitude,
			}
			if err = CreateGateway(db, &gw); err != nil {
				return errors.Wrap(err, "create gateway error")
			}
		} else {
			return errors.Wrap(err, "get gateway error")
		}
	} else {
		// update the gateway
		now := time.Now()
		if gw.FirstSeenAt == nil {
			gw.FirstSeenAt = &now
		}
		gw.LastSeenAt = &now

		if stats.Location != nil {
			gw.Location = location
			gw.Altitude = altitude
		}

		if err = UpdateGateway(db, &gw); err != nil {
			return errors.Wrap(err, "update gateway error")
		}
	}

	if err := handleConfigurationUpdate(db, gw, stats.ConfigVersion); err != nil {
		log.WithError(err).WithField("gateway_id", gw.GatewayID).Error("handle gateway-configuration update error")
	}

	comitted := false
	tx, err := db.Beginx()
	if err != nil {
		return errors.Wrap(err, "begin transaction error")
	}
	defer func() {
		if !comitted {
			tx.Rollback()
		}
	}()

	// set the database timezone for this transaction
	if config.C.NetworkServer.Gateway.Stats.TimezoneLocation != time.Local {
		// when TimeLocation == time.Local, it would have 'Local' as name
		_, err = tx.Exec(fmt.Sprintf("set local time zone '%s'", config.C.NetworkServer.Gateway.Stats.TimezoneLocation.String()))
		if err != nil {
			return errors.Wrap(err, "set timezone error")
		}
	}

	ts, err := ptypes.Timestamp(stats.Time)
	if err != nil {
		return errors.Wrap(err, "timestamp error")
	}

	// store the stats
	for _, aggr := range statsAggregationIntervals {
		if err := aggregateGatewayStats(tx, Stats{
			GatewayID:           gatewayID,
			Timestamp:           ts,
			Interval:            aggr,
			RXPacketsReceived:   int(stats.RxPacketsReceived),
			RXPacketsReceivedOK: int(stats.RxPacketsReceivedOk),
			TXPacketsReceived:   int(stats.TxPacketsReceived),
			TXPacketsEmitted:    int(stats.TxPacketsEmitted),
		}); err != nil {
			return errors.Wrap(err, "aggregate gateway stats error")
		}
	}

	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, "commit error")
	}
	comitted = true
	return nil
}

func aggregateGatewayStats(db sqlx.Execer, stats Stats) error {
	_, err := db.Exec(`
		insert into gateway_stats (
			gateway_id,
			"timestamp",
			"interval",
			rx_packets_received,
			rx_packets_received_ok,
			tx_packets_received,
			tx_packets_emitted
		) values (
			$1,
			cast(date_trunc($2, $3::timestamptz) as timestamp with time zone),
			$2,
			$4,
			$5,
			$6,
			$7
		)
		on conflict (gateway_id, "timestamp", "interval")
			do update set
				rx_packets_received = gateway_stats.rx_packets_received + $4,
				rx_packets_received_ok = gateway_stats.rx_packets_received_ok + $5,
				tx_packets_received = gateway_stats.tx_packets_received + $6,
				tx_packets_emitted = gateway_stats.tx_packets_emitted + $7`,
		stats.GatewayID[:],
		stats.Interval,
		stats.Timestamp,
		stats.RXPacketsReceived,
		stats.RXPacketsReceivedOK,
		stats.TXPacketsReceived,
		stats.TXPacketsEmitted,
	)
	if err != nil {
		return errors.Wrap(err, "insert or update aggregate error")
	}
	return nil
}

func handleConfigurationUpdate(db sqlx.Queryer, g Gateway, currentVersion string) error {
	if g.GatewayProfileID == nil {
		log.WithField("gateway_id", g.GatewayID).Debug("gateway-profile is not set, skipping configuration update")
		return nil
	}

	gwProfile, err := GetGatewayProfile(db, *g.GatewayProfileID)
	if err != nil {
		return errors.Wrap(err, "get gateway-profile error")
	}

	if gwProfile.GetVersion() == currentVersion {
		log.WithFields(log.Fields{
			"gateway_id": g.GatewayID,
			"version":    currentVersion,
		}).Debug("gateway configuration is up-to-date")
		return nil
	}

	configPacket := gw.GatewayConfiguration{
		GatewayId: g.GatewayID[:],
		Version:   gwProfile.GetVersion(),
	}

	for _, i := range gwProfile.Channels {
		c, err := config.C.NetworkServer.Band.Band.GetUplinkChannel(int(i))
		if err != nil {
			return errors.Wrap(err, "get channel error")
		}

		gwC := gw.ChannelConfiguration{
			Frequency:  uint32(c.Frequency),
			Modulation: commonPB.Modulation_LORA,
		}

		modConfig := gw.LoRaModulationConfig{}

		for drI := c.MaxDR; drI >= c.MinDR; drI-- {
			dr, err := config.C.NetworkServer.Band.Band.GetDataRate(drI)
			if err != nil {
				return errors.Wrap(err, "get data-rate error")
			}

			modConfig.SpreadingFactors = append(modConfig.SpreadingFactors, uint32(dr.SpreadFactor))
			modConfig.Bandwidth = uint32(dr.Bandwidth)
		}

		gwC.ModulationConfig = &gw.ChannelConfiguration_LoraModulationConfig{
			LoraModulationConfig: &modConfig,
		}

		configPacket.Channels = append(configPacket.Channels, &gwC)
	}

	for _, c := range gwProfile.ExtraChannels {
		gwC := gw.ChannelConfiguration{
			Frequency: uint32(c.Frequency),
		}

		switch band.Modulation(c.Modulation) {
		case band.LoRaModulation:
			gwC.Modulation = commonPB.Modulation_LORA
			modConfig := gw.LoRaModulationConfig{
				Bandwidth: uint32(c.Bandwidth),
			}

			for _, sf := range c.SpreadingFactors {
				modConfig.SpreadingFactors = append(modConfig.SpreadingFactors, uint32(sf))
			}

			gwC.ModulationConfig = &gw.ChannelConfiguration_LoraModulationConfig{
				LoraModulationConfig: &modConfig,
			}
		case band.FSKModulation:
			gwC.Modulation = commonPB.Modulation_FSK
			modConfig := gw.FSKModulationConfig{
				Bandwidth: uint32(c.Bandwidth),
				Bitrate:   uint32(c.Bitrate),
			}

			gwC.ModulationConfig = &gw.ChannelConfiguration_FskModulationConfig{
				FskModulationConfig: &modConfig,
			}
		}

		configPacket.Channels = append(configPacket.Channels, &gwC)
	}

	if err := config.C.NetworkServer.Gateway.Backend.Backend.SendGatewayConfigPacket(configPacket); err != nil {
		return errors.Wrap(err, "send gateway-configuration packet error")
	}

	return nil
}
