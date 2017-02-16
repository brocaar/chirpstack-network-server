package gateway

import (
	"database/sql/driver"
	"fmt"
	"strconv"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/jmoiron/sqlx"

	"github.com/brocaar/lorawan"
)

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

// Gateway represents a single gateway.
type Gateway struct {
	MAC                 lorawan.EUI64 `db:"mac"`
	CratedAt            time.Time     `db:"created_at"`
	UpdatedAt           time.Time     `db:"updated_at"`
	Location            GPSPoint      `db:"location"`
	Altitude            int           `db:"altitude"`
	RXPacketsReceived   int64         `db:"rx_packets_received"`
	RXPacketsReceivedOK int64         `db:"rx_packets_received_ok"`
	TXPacketsReceived   int64         `db:"tx_packets_received"`
	TXPacketsEmitted    int64         `db:"tx_packets_emitted"`
}

// CreateGateway creates the given gateway.
func CreateGateway(db *sqlx.DB, gw *Gateway) error {
	now := time.Now()
	_, err := db.Exec(`
		insert into gateway (
			mac,
			created_at,
			updated_at,
			location,
			altitude,
			rx_packets_received,
			rx_packets_received_ok,
			tx_packets_received,
			tx_packets_emitted
		) values ($1, $2, $2, $3, $4, $5, $6, $7, $8)`,
		gw.MAC[:],
		now,
		gw.Location,
		gw.Altitude,
		gw.RXPacketsReceived,
		gw.RXPacketsReceivedOK,
		gw.TXPacketsReceived,
		gw.TXPacketsEmitted,
	)
	if err != nil {
		return fmt.Errorf("create gateway error: %s", err)
	}
	gw.CratedAt = now
	gw.UpdatedAt = now
	log.WithField("mac", gw.MAC).Info("gateway created")
	return nil
}

// GetGateway returns the gateway for the given MAC.
func GetGateway(db *sqlx.DB, mac lorawan.EUI64) (Gateway, error) {
	var gw Gateway
	err := db.Get(&gw, "select * from gateway where mac = $1", mac[:])
	if err != nil {
		return gw, fmt.Errorf("get gateway error: %s", err)
	}
	return gw, nil
}

// UpdateGateway updates the given gateway.
func UpdateGateway(db *sqlx.DB, gw *Gateway) error {
	now := time.Now()
	res, err := db.Exec(`
		update gateway set
			updated_at = $2,
			location = $3,
			altitude = $4,
			rx_packets_received = $5,
			rx_packets_received_ok = $6,
			tx_packets_received = $7,
			tx_packets_emitted = $8
		where mac = $1`,
		gw.MAC[:],
		now,
		gw.Location,
		gw.Altitude,
		gw.RXPacketsReceived,
		gw.RXPacketsReceivedOK,
		gw.TXPacketsReceived,
		gw.TXPacketsEmitted,
	)
	if err != nil {
		return fmt.Errorf("update gateway error: %s", err)
	}
	ra, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if ra == 0 {
		return fmt.Errorf("gateway %s does not exist", gw.MAC)
	}
	gw.UpdatedAt = now
	log.WithField("mac", gw.MAC).Info("gateway updated")
	return nil
}

// DeleteGateway deletes the gateway matching the given MAC.
func DeleteGateway(db *sqlx.DB, mac lorawan.EUI64) error {
	res, err := db.Exec("delete from gateway where mac = $1", mac[:])
	if err != nil {
		return fmt.Errorf("delete gateway error: %s", err)
	}
	ra, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if ra == 0 {
		return fmt.Errorf("gateway %s does not exist", mac)
	}
	log.WithField("mac", mac).Info("gateway deleted")
	return nil
}

// GetGatewayCount returns the number of gateways.
func GetGatewayCount(db *sqlx.DB) (int, error) {
	var count int
	err := db.Get(&count, "select count(*) from gateway")
	if err != nil {
		return 0, fmt.Errorf("get gateway count error: %s", err)
	}
	return count, nil
}

// GetGateways returns a slice of gateways, order by mac and respecting the
// given limit and offset.
func GetGateways(db *sqlx.DB, limit, offset int) ([]Gateway, error) {
	var gws []Gateway
	err := db.Select(&gws, "select * from gateway order by mac limit $1 offset $2", limit, offset)
	if err != nil {
		return nil, fmt.Errorf("get gateways error: %s", err)
	}
	return gws, nil
}
