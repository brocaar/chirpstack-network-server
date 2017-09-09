package storage

import (
	"time"

	"github.com/jmoiron/sqlx"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/lorawan"
)

// Device defines a LoRaWAN device.
type Device struct {
	DevEUI           lorawan.EUI64 `db:"dev_eui"`
	CreatedBy        string        `db:"created_by"`
	CreatedAt        time.Time     `db:"created_at"`
	UpdatedAt        time.Time     `db:"updated_at"`
	DeviceProfileID  string        `db:"device_profile_id"`
	ServiceProfileID string        `db:"service_profile_id"`
	RoutingProfileID string        `db:"routing_profile_id"`
}

// CreateDevice creates the given device.
func CreateDevice(db *sqlx.DB, d *Device) error {
	now := time.Now()
	d.CreatedAt = now
	d.UpdatedAt = now

	_, err := db.Exec(`
		insert into device (
			dev_eui,
			created_by,
			created_at,
			updated_at,
			device_profile_id,
			service_profile_id,
			routing_profile_id
		) values ($1, $2, $3, $4, $5, $6, $7)`,
		d.DevEUI[:],
		d.CreatedBy,
		d.CreatedAt,
		d.UpdatedAt,
		d.DeviceProfileID,
		d.ServiceProfileID,
		d.RoutingProfileID,
	)
	if err != nil {
		return handlePSQLError(err, "insert error")
	}

	log.WithFields(log.Fields{
		"created_by": d.CreatedBy,
		"dev_eui":    d.DevEUI,
	}).Info("device created")

	return nil
}

// GetDevice returns the device matching the given DevEUI.
func GetDevice(db *sqlx.DB, devEUI lorawan.EUI64) (Device, error) {
	var d Device
	err := db.Get(&d, "select * from device where dev_eui = $1", devEUI[:])
	if err != nil {
		return d, handlePSQLError(err, "select error")
	}

	return d, nil
}

// UpdateDevice updates the given device.
func UpdateDevice(db *sqlx.DB, d *Device) error {
	d.UpdatedAt = time.Now()
	res, err := db.Exec(`
		update device set
			updated_at = $2,
			device_profile_id = $3,
			service_profile_id = $4,
			routing_profile_id = $5
		where
			dev_eui = $1`,
		d.DevEUI[:],
		d.UpdatedAt,
		d.DeviceProfileID,
		d.ServiceProfileID,
		d.RoutingProfileID,
	)
	if err != nil {
		return handlePSQLError(err, "update error")
	}
	ra, err := res.RowsAffected()
	if err != nil {
		return handlePSQLError(err, "get rows affected error")
	}
	if ra == 0 {
		return ErrDoesNotExist
	}

	log.WithField("dev_eui", d.DevEUI).Info("device updated")
	return nil
}

// DeleteDevice deletes the device matching the given DevEUI.
func DeleteDevice(db *sqlx.DB, devEUI lorawan.EUI64) error {
	res, err := db.Exec("delete from device where dev_eui = $1", devEUI[:])
	if err != nil {
		return handlePSQLError(err, "delete error")
	}

	ra, err := res.RowsAffected()
	if err != nil {
		return handlePSQLError(err, "get rows affected error")
	}
	if ra == 0 {
		return ErrDoesNotExist
	}

	log.WithField("dev_eui", devEUI).Info("node deleted")
	return nil
}
