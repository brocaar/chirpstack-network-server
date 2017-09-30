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

// DeviceActivation defines the device-activation for a LoRaWAN device.
type DeviceActivation struct {
	ID        int64             `db:"id"`
	CreatedAt time.Time         `db:"created_at"`
	DevEUI    lorawan.EUI64     `db:"dev_eui"`
	JoinEUI   lorawan.EUI64     `db:"join_eui"`
	DevAddr   lorawan.DevAddr   `db:"dev_addr"`
	NwkSKey   lorawan.AES128Key `db:"nwk_s_key"`
	DevNonce  lorawan.DevNonce  `db:"dev_nonce"`
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

// CreateDeviceActivation creates the given device-activation.
func CreateDeviceActivation(db sqlx.Queryer, da *DeviceActivation) error {
	da.CreatedAt = time.Now()

	err := sqlx.Get(db, &da.ID, `
		insert into device_activation (
			created_at,
			dev_eui,
			join_eui,
			dev_addr,
			nwk_s_key,
			dev_nonce
		) values ($1, $2, $3, $4, $5, $6)
		returning id`,
		da.CreatedAt,
		da.DevEUI[:],
		da.JoinEUI[:],
		da.DevAddr[:],
		da.NwkSKey[:],
		da.DevNonce[:],
	)
	if err != nil {
		return handlePSQLError(err, "insert error")
	}

	log.WithFields(log.Fields{
		"id":      da.ID,
		"dev_eui": da.DevEUI,
	}).Info("device-activation created")

	return nil
}

// GetLastDeviceActivationForDevEUI returns the most recent activation
// for the given DevEUI.
func GetLastDeviceActivationForDevEUI(db sqlx.Queryer, devEUI lorawan.EUI64) (DeviceActivation, error) {
	var da DeviceActivation
	err := sqlx.Get(db, &da, `
		select
			*
		from device_activation
		where
			dev_eui = $1
		order by
			created_at desc
		limit 1`,
		devEUI[:],
	)
	if err != nil {
		return da, handlePSQLError(err, "select error")
	}

	return da, nil
}

// ValidateDevNonce validates the given dev-nonce for the given
// DevEUI / JoinEUI combination.
func ValidateDevNonce(db sqlx.Queryer, joinEUI, devEUI lorawan.EUI64, nonce lorawan.DevNonce) error {
	var count int
	err := sqlx.Get(db, &count, `
		select
			count(*)
		from
			device_activation
		where
			dev_eui = $1
			and join_eui = $2
			and dev_nonce = $3`,
		devEUI,
		joinEUI,
		nonce,
	)
	if err != nil {
		return handlePSQLError(err, "select error")
	}

	if count != 0 {
		return ErrAlreadyExists
	}

	return nil
}
