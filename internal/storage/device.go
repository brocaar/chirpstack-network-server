package storage

import (
	"context"
	"time"

	"github.com/gofrs/uuid"
	"github.com/jmoiron/sqlx"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/chirpstack-network-server/internal/logging"
	"github.com/brocaar/lorawan"
)

// DeviceMode defines the mode in which the device operates.
type DeviceMode string

// Available device modes.
const (
	DeviceModeA DeviceMode = "A"
	DeviceModeB DeviceMode = "B"
	DeviceModeC DeviceMode = "C"
)

// Device defines a LoRaWAN device.
type Device struct {
	DevEUI            lorawan.EUI64 `db:"dev_eui"`
	CreatedAt         time.Time     `db:"created_at"`
	UpdatedAt         time.Time     `db:"updated_at"`
	DeviceProfileID   uuid.UUID     `db:"device_profile_id"`
	ServiceProfileID  uuid.UUID     `db:"service_profile_id"`
	RoutingProfileID  uuid.UUID     `db:"routing_profile_id"`
	SkipFCntCheck     bool          `db:"skip_fcnt_check"`
	ReferenceAltitude float64       `db:"reference_altitude"`
	Mode              DeviceMode    `db:"mode"`
	IsDisabled        bool          `db:"is_disabled"`
}

// DeviceActivation defines the device-activation for a LoRaWAN device.
type DeviceActivation struct {
	ID          int64             `db:"id"`
	CreatedAt   time.Time         `db:"created_at"`
	DevEUI      lorawan.EUI64     `db:"dev_eui"`
	JoinEUI     lorawan.EUI64     `db:"join_eui"`
	DevAddr     lorawan.DevAddr   `db:"dev_addr"`
	FNwkSIntKey lorawan.AES128Key `db:"f_nwk_s_int_key"`
	SNwkSIntKey lorawan.AES128Key `db:"s_nwk_s_int_key"`
	NwkSEncKey  lorawan.AES128Key `db:"nwk_s_enc_key"`
	DevNonce    lorawan.DevNonce  `db:"dev_nonce"`
	JoinReqType lorawan.JoinType  `db:"join_req_type"`
}

// CreateDevice creates the given device.
func CreateDevice(ctx context.Context, db sqlx.Execer, d *Device) error {
	now := time.Now()
	d.CreatedAt = now
	d.UpdatedAt = now

	_, err := db.Exec(`
		insert into device (
			dev_eui,
			created_at,
			updated_at,
			device_profile_id,
			service_profile_id,
			routing_profile_id,
			skip_fcnt_check,
			reference_altitude,
			mode,
			is_disabled
		) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		d.DevEUI[:],
		d.CreatedAt,
		d.UpdatedAt,
		d.DeviceProfileID,
		d.ServiceProfileID,
		d.RoutingProfileID,
		d.SkipFCntCheck,
		d.ReferenceAltitude,
		d.Mode,
		d.IsDisabled,
	)
	if err != nil {
		return handlePSQLError(err, "insert error")
	}

	log.WithFields(log.Fields{
		"dev_eui": d.DevEUI,
		"ctx_id":  ctx.Value(logging.ContextIDKey),
	}).Info("device created")

	return nil
}

// GetDevice returns the device matching the given DevEUI.
func GetDevice(ctx context.Context, db sqlx.Queryer, devEUI lorawan.EUI64, forUpdate bool) (Device, error) {
	var d Device
	var fu string

	if forUpdate {
		fu = " for update"
	}

	err := sqlx.Get(db, &d, "select * from device where dev_eui = $1"+fu, devEUI[:])
	if err != nil {
		return d, handlePSQLError(err, "select error")
	}

	return d, nil
}

// UpdateDevice updates the given device.
func UpdateDevice(ctx context.Context, db sqlx.Execer, d *Device) error {
	d.UpdatedAt = time.Now()
	res, err := db.Exec(`
		update device set
			updated_at = $2,
			device_profile_id = $3,
			service_profile_id = $4,
			routing_profile_id = $5,
			skip_fcnt_check = $6,
			reference_altitude = $7,
			mode = $8,
			is_disabled = $9
		where
			dev_eui = $1`,
		d.DevEUI[:],
		d.UpdatedAt,
		d.DeviceProfileID,
		d.ServiceProfileID,
		d.RoutingProfileID,
		d.SkipFCntCheck,
		d.ReferenceAltitude,
		d.Mode,
		d.IsDisabled,
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

	log.WithFields(log.Fields{
		"dev_eui": d.DevEUI,
		"ctx_id":  ctx.Value(logging.ContextIDKey),
	}).Info("device updated")
	return nil
}

// DeleteDevice deletes the device matching the given DevEUI.
func DeleteDevice(ctx context.Context, db sqlx.Execer, devEUI lorawan.EUI64) error {
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

	log.WithFields(log.Fields{
		"dev_eui": devEUI,
		"ctx_id":  ctx.Value(logging.ContextIDKey),
	}).Info("device deleted")
	return nil
}

// CreateDeviceActivation creates the given device-activation.
func CreateDeviceActivation(ctx context.Context, db sqlx.Queryer, da *DeviceActivation) error {
	da.CreatedAt = time.Now()

	err := sqlx.Get(db, &da.ID, `
		insert into device_activation (
			created_at,
			dev_eui,
			join_eui,
			dev_addr,
			s_nwk_s_int_key,
			f_nwk_s_int_key,
			nwk_s_enc_key,
			dev_nonce,
			join_req_type
		) values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		returning id`,
		da.CreatedAt,
		da.DevEUI[:],
		da.JoinEUI[:],
		da.DevAddr[:],
		da.SNwkSIntKey[:],
		da.FNwkSIntKey[:],
		da.NwkSEncKey[:],
		da.DevNonce,
		da.JoinReqType,
	)
	if err != nil {
		return handlePSQLError(err, "insert error")
	}

	log.WithFields(log.Fields{
		"id":      da.ID,
		"dev_eui": da.DevEUI,
		"ctx_id":  ctx.Value(logging.ContextIDKey),
	}).Info("device-activation created")

	return nil
}

// DeleteDeviceActivationsForDevice removes the device-activation for the given
// DevEUI.
func DeleteDeviceActivationsForDevice(ctx context.Context, db sqlx.Execer, devEUI lorawan.EUI64) error {
	_, err := db.Exec(`
		delete
		from
			device_activation
		where
			dev_eui = $1
	`, devEUI[:])
	if err != nil {
		return handlePSQLError(err, "delete error")
	}

	log.WithFields(log.Fields{
		"dev_eui": devEUI,
		"ctx_id":  ctx.Value(logging.ContextIDKey),
	}).Info("device-activations deleted")
	return nil
}

// GetLastDeviceActivationForDevEUI returns the most recent activation
// for the given DevEUI.
func GetLastDeviceActivationForDevEUI(ctx context.Context, db sqlx.Queryer, devEUI lorawan.EUI64) (DeviceActivation, error) {
	var da DeviceActivation
	err := sqlx.Get(db, &da, `
		select
			*
		from device_activation
		where
			dev_eui = $1
		order by
			id desc
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
func ValidateDevNonce(ctx context.Context, db sqlx.Queryer, joinEUI, devEUI lorawan.EUI64, nonce lorawan.DevNonce, joinType lorawan.JoinType) error {
	var count int
	err := sqlx.Get(db, &count, `
		select
			count(*)
		from
			device_activation
		where
			dev_eui = $1
			and join_eui = $2
			and dev_nonce = $3
			and join_req_type = $4`,
		devEUI,
		joinEUI,
		nonce,
		joinType,
	)
	if err != nil {
		return handlePSQLError(err, "select error")
	}

	if count != 0 {
		return ErrAlreadyExists
	}

	return nil
}
