package gateway

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/pkg/errors"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/lorawan"
)

var gatewayNameRegexp = regexp.MustCompile(`^[\w-]+$`)

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

// GPSPoint contains a GPS point.
type GPSPoint struct {
	Latitude  float64
	Longitude float64
}

// Value implements the driver.Valuer interface.
func (l GPSPoint) Value() (driver.Value, error) {
	return fmt.Sprintf("(%s,%s)", strconv.FormatFloat(l.Latitude, 'f', -1, 64), strconv.FormatFloat(l.Longitude, 'f', -1, 64)), nil
}

// Scan implements the sql.Scanner interface.
func (l *GPSPoint) Scan(src interface{}) error {
	b, ok := src.([]byte)
	if !ok {
		return fmt.Errorf("expected []byte, got %T", src)
	}

	_, err := fmt.Sscanf(string(b), "(%f,%f)", &l.Latitude, &l.Longitude)
	return err
}

// StatsHandler represents a stat handler for incoming gateway stats.
type StatsHandler struct {
	ctx common.Context
	wg  sync.WaitGroup
}

// NewStatsHandler creates a new StatsHandler.
func NewStatsHandler(ctx common.Context) *StatsHandler {
	return &StatsHandler{
		ctx: ctx,
	}
}

// Start starts the stats handler.
func (s *StatsHandler) Start() error {
	go func() {
		s.wg.Add(1)
		defer s.wg.Done()
		handleStatsPackets(&s.wg, s.ctx)
	}()
	return nil
}

// Stop waits for the stats handler to complete the pending packets.
// At this stage the gateway backend must already been closed.
func (s *StatsHandler) Stop() error {
	s.wg.Wait()
	return nil
}

// Gateway represents a single gateway.
type Gateway struct {
	MAC         lorawan.EUI64 `db:"mac"`
	Name        string        `db:"name"`
	Description string        `db:"description"`
	CreatedAt   time.Time     `db:"created_at"`
	UpdatedAt   time.Time     `db:"updated_at"`
	FirstSeenAt *time.Time    `db:"first_seen_at"`
	LastSeenAt  *time.Time    `db:"last_seen_at"`
	Location    *GPSPoint     `db:"location"`
	Altitude    *float64      `db:"altitude"`
}

// Validate validates the data of the gateway.
func (g Gateway) Validate() error {
	if !gatewayNameRegexp.MatchString(g.Name) {
		return ErrInvalidName
	}
	return nil
}

// Stats represents a single gateway stats record.
type Stats struct {
	MAC                 lorawan.EUI64 `db:"mac"`
	Timestamp           time.Time     `db:"timestamp"`
	Interval            string        `db:"interval"`
	RXPacketsReceived   int           `db:"rx_packets_received"`
	RXPacketsReceivedOK int           `db:"rx_packets_received_ok"`
	TXPacketsReceived   int           `db:"tx_packets_received"`
	TXPacketsEmitted    int           `db:"tx_packets_emitted"`
}

// CreateGateway creates the given gateway.
func CreateGateway(db *sqlx.DB, gw *Gateway) error {
	if err := gw.Validate(); err != nil {
		return errors.Wrap(err, "validate error")
	}

	now := time.Now()
	_, err := db.Exec(`
		insert into gateway (
			mac,
			name,
			description,
			created_at,
			updated_at,
			first_seen_at,
			last_seen_at,
			location,
			altitude
		) values ($1, $2, $3, $4, $4, $5, $6, $7, $8)`,
		gw.MAC[:],
		gw.Name,
		gw.Description,
		now,
		gw.FirstSeenAt,
		gw.LastSeenAt,
		gw.Location,
		gw.Altitude,
	)
	if err != nil {
		switch err := err.(type) {
		case *pq.Error:
			switch err.Code.Name() {
			case "unique_violation":
				return ErrAlreadyExists
			default:
				return errors.Wrap(err, "insert error")
			}
		default:
			return errors.Wrap(err, "insert error")
		}
	}
	gw.CreatedAt = now
	gw.UpdatedAt = now
	log.WithField("mac", gw.MAC).Info("gateway created")
	return nil
}

// GetGateway returns the gateway for the given MAC.
func GetGateway(db *sqlx.DB, mac lorawan.EUI64) (Gateway, error) {
	var gw Gateway
	err := db.Get(&gw, "select * from gateway where mac = $1", mac[:])
	if err != nil {
		if err == sql.ErrNoRows {
			return gw, ErrDoesNotExist
		}
		return gw, errors.Wrap(err, "select error")
	}
	return gw, nil
}

// UpdateGateway updates the given gateway.
func UpdateGateway(db *sqlx.DB, gw *Gateway) error {
	if err := gw.Validate(); err != nil {
		return errors.Wrap(err, "validate error")
	}

	now := time.Now()
	res, err := db.Exec(`
		update gateway set
			name = $2,
			description = $3,
			updated_at = $4,
			first_seen_at = $5,
			last_seen_at = $6,
			location = $7,
			altitude = $8
		where mac = $1`,
		gw.MAC[:],
		gw.Name,
		gw.Description,
		now,
		gw.FirstSeenAt,
		gw.LastSeenAt,
		gw.Location,
		gw.Altitude,
	)
	if err != nil {
		return errors.Wrap(err, "update error")
	}
	ra, err := res.RowsAffected()
	if err != nil {
		return errors.Wrap(err, "get rows affected error")
	}
	if ra == 0 {
		return ErrDoesNotExist
	}
	gw.UpdatedAt = now
	log.WithField("mac", gw.MAC).Info("gateway updated")
	return nil
}

// DeleteGateway deletes the gateway matching the given MAC.
func DeleteGateway(db *sqlx.DB, mac lorawan.EUI64) error {
	res, err := db.Exec("delete from gateway where mac = $1", mac[:])
	if err != nil {
		return errors.Wrap(err, "delete error")
	}
	ra, err := res.RowsAffected()
	if err != nil {
		return errors.Wrap(err, "get rows affected error")
	}
	if ra == 0 {
		return ErrDoesNotExist
	}
	log.WithField("mac", mac).Info("gateway deleted")
	return nil
}

// GetGatewayCount returns the number of gateways.
func GetGatewayCount(db *sqlx.DB) (int, error) {
	var count int
	err := db.Get(&count, "select count(*) from gateway")
	if err != nil {
		return 0, errors.Wrap(err, "select error")
	}
	return count, nil
}

// GetGateways returns a slice of gateways, order by mac and respecting the
// given limit and offset.
func GetGateways(db *sqlx.DB, limit, offset int) ([]Gateway, error) {
	var gws []Gateway
	err := db.Select(&gws, "select * from gateway order by mac limit $1 offset $2", limit, offset)
	if err != nil {
		return nil, errors.Wrap(err, "select error")
	}
	return gws, nil
}

// GetGatewaysForMACs returns a map of gateways given a slice of MACs.
func GetGatewaysForMACs(db *sqlx.DB, macs []lorawan.EUI64) (map[lorawan.EUI64]Gateway, error) {
	out := make(map[lorawan.EUI64]Gateway)
	var macsB [][]byte
	for i := range macs {
		macsB = append(macsB, macs[i][:])
	}

	var gws []Gateway
	err := db.Select(&gws, "select * from gateway where mac = any($1)", pq.ByteaArray(macsB))
	if err != nil {
		return nil, errors.Wrap(err, "select error")
	}

	if len(gws) != len(macs) {
		return nil, fmt.Errorf("expected %d gateways, got %d", len(macs), len(out))
	}

	for i := range gws {
		out[gws[i].MAC] = gws[i]
	}

	return out, nil
}

// GetGatewayStats returns the stats for the given gateway.
// Note that the stats will return a record for each interval.
func GetGatewayStats(db *sqlx.DB, mac lorawan.EUI64, interval string, start, end time.Time) ([]Stats, error) {
	var valid bool
	interval = strings.ToUpper(interval)
	zone, _ := start.In(common.TimeLocation).Zone()

	for _, i := range statsAggregationIntervals {
		if i == interval {
			valid = true
		}
	}

	if !valid {
		return nil, ErrInvalidAggregationInterval
	}

	var stats []Stats

	err := db.Select(&stats, `
		select
			$1::bytea as mac,
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
				mac = $1
				and interval = $2
				and "timestamp" >= cast(date_trunc($2, $3 at time zone $4) as timestamp) at time zone $4
				and "timestamp" < $5) gs
		right join (
			select generate_series(
				cast(date_trunc($2, $3 at time zone $4) as timestamp) at time zone $4,
				$5,
				$6) as "timestamp"
			) s
			on gs.timestamp = s.timestamp
		order by s.timestamp`,
		mac[:],
		interval,
		start,
		zone,
		end,
		fmt.Sprintf("1 %s", interval),
	)
	if err != nil {
		return nil, errors.Wrap(err, "select error")
	}
	return stats, nil
}

// handleStatsPackets consumes received stats packets by the gateway.
func handleStatsPackets(wg *sync.WaitGroup, ctx common.Context) {
	for statsPacket := range ctx.Gateway.StatsPacketChan() {
		go func(stats gw.GatewayStatsPacket) {
			wg.Add(1)
			defer wg.Done()
			if err := handleStatsPacket(ctx.DB, stats); err != nil {
				log.Errorf("handle stats packet error: %s", err)
			}
		}(statsPacket)
	}
}

// handleStatsPacket handles a received stats packet by the gateway.
func handleStatsPacket(db *sqlx.DB, stats gw.GatewayStatsPacket) error {
	var location *GPSPoint
	var altitude *float64

	if stats.Latitude != nil && stats.Longitude != nil {
		location = &GPSPoint{
			Latitude:  *stats.Latitude,
			Longitude: *stats.Longitude,
		}
	}

	if stats.Altitude != nil {
		altitude = stats.Altitude
	}

	// create or update the gateway
	gw, err := GetGateway(db, stats.MAC)
	if err != nil {
		// create the gateway
		if err == ErrDoesNotExist && common.CreateGatewayOnStats {
			now := time.Now()

			gw = Gateway{
				MAC:         stats.MAC,
				Name:        stats.MAC.String(),
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
		if location != nil {
			gw.Location = location
		}
		if altitude != nil {
			gw.Altitude = altitude
		}

		if err = UpdateGateway(db, &gw); err != nil {
			return errors.Wrap(err, "update gateway error")
		}
	}

	// store the stats
	for _, aggr := range statsAggregationIntervals {
		if err := aggregateGatewayStats(db, Stats{
			MAC:                 stats.MAC,
			Timestamp:           stats.Time,
			Interval:            aggr,
			RXPacketsReceived:   stats.RXPacketsReceived,
			RXPacketsReceivedOK: stats.RXPacketsReceivedOK,
			TXPacketsReceived:   stats.TXPacketsReceived,
			TXPacketsEmitted:    stats.TXPacketsEmitted,
		}); err != nil {
			return errors.Wrap(err, "aggregate gateway stats error")
		}
	}

	return nil
}

func aggregateGatewayStats(db *sqlx.DB, stats Stats) error {
	zone, _ := stats.Timestamp.In(common.TimeLocation).Zone()

	_, err := db.Exec(`
		insert into gateway_stats (
			mac,
			"timestamp",
			"interval",
			rx_packets_received,
			rx_packets_received_ok,
			tx_packets_received,
			tx_packets_emitted
		) values (
			$1,
			cast(date_trunc($2, $3 at time zone $4) as timestamp) at time zone $4,
			$2,
			$5,
			$6,
			$7,
			$8
		)
		on conflict (mac, "timestamp", "interval")
			do update set
				rx_packets_received = gateway_stats.rx_packets_received + $5,
				rx_packets_received_ok = gateway_stats.rx_packets_received_ok + $6,
				tx_packets_received = gateway_stats.tx_packets_received + $7,
				tx_packets_emitted = gateway_stats.tx_packets_emitted + $8`,
		stats.MAC[:],
		stats.Interval,
		stats.Timestamp,
		zone,
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
