package code

import (
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/lorawan"
)

// MigrateGatewayStats migrates the gateway stats from PostgreSQL to Redis.
func MigrateGatewayStats(p *redis.Pool, db sqlx.Queryer) error {
	log.Info("migrating gateway stats")

	var row struct {
		GatewayID           lorawan.EUI64               `db:"gateway_id"`
		Timestamp           time.Time                   `db:"timestamp"`
		Interval            storage.AggregationInterval `db:"interval"`
		RXPacketsReceived   int                         `db:"rx_packets_received"`
		RXPacketsReceivedOK int                         `db:"rx_packets_received_ok"`
		TXPacketsReceived   int                         `db:"tx_packets_received"`
		TXPacketsEmitted    int                         `db:"tx_packets_emitted"`
	}

	rows, err := db.Queryx(`
		select
			gateway_id,
			timestamp,
			interval,
			rx_packets_received,
			rx_packets_received_ok,
			tx_packets_received,
			tx_packets_emitted
		from
			gateway_stats
		where
			(interval = 'MINUTE' and timestamp >= (now() - interval '65 minutes')) or
			(interval = 'HOUR' and timestamp >= (now() - interval '25 hours')) or
			(interval = 'DAY' and timestamp >= (now() - interval '32 days')) or
			(interval = 'MONTH' and timestamp >= (now() - interval '13 months'))
	`)
	if err != nil {
		return errors.Wrap(err, "select error")
	}
	defer rows.Close()

	for rows.Next() {
		if err := rows.StructScan(&row); err != nil {
			return errors.Wrap(err, "scan error")
		}

		switch row.Interval {
		case storage.AggregationMinute, storage.AggregationHour, storage.AggregationDay, storage.AggregationMonth:
			err = storage.SaveMetricsForInterval(p, row.Interval, "gw:"+row.GatewayID.String(), storage.MetricsRecord{
				Time: row.Timestamp,
				Metrics: map[string]float64{
					"rx_count":    float64(row.RXPacketsReceived),
					"rx_ok_count": float64(row.RXPacketsReceivedOK),
					"tx_count":    float64(row.TXPacketsReceived),
					"tx_ok_count": float64(row.TXPacketsEmitted),
				},
			})
			if err != nil {
				return errors.Wrap(err, "save metrics error")
			}
		}
	}

	log.Info("gateway stats migrated")
	return nil
}
