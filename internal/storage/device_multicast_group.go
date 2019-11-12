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

// AddDeviceToMulticastGroup adds the given device to the given multicast-group.
func AddDeviceToMulticastGroup(ctx context.Context, db sqlx.Execer, devEUI lorawan.EUI64, multicastGroupID uuid.UUID) error {
	_, err := db.Exec(`
		insert into device_multicast_group (
			dev_eui,
			multicast_group_id,
			created_at
		) values ($1, $2, $3)`,
		devEUI, multicastGroupID, time.Now())
	if err != nil {
		return handlePSQLError(err, "insert error")
	}

	log.WithFields(log.Fields{
		"dev_eui":            devEUI,
		"multicast_group_id": multicastGroupID,
		"ctx_id":             ctx.Value(logging.ContextIDKey),
	}).Info("device added to multicast-group")

	return nil
}

// RemoveDeviceFromMulticastGroup removes the given device from the given
// multicast-group.
func RemoveDeviceFromMulticastGroup(ctx context.Context, db sqlx.Execer, devEUI lorawan.EUI64, multicastGroupID uuid.UUID) error {
	res, err := db.Exec(`
		delete from
			device_multicast_group
		where
			dev_eui = $1
			and multicast_group_id = $2`,
		devEUI[:],
		multicastGroupID,
	)
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
		"dev_eui":            devEUI,
		"multicast_group_id": multicastGroupID,
		"ctx_id":             ctx.Value(logging.ContextIDKey),
	}).Info("device removed from multicast-group")

	return nil
}

// GetMulticastGroupsForDevEUI returns the multicast-group ids to which the
// given Device EUI belongs.
func GetMulticastGroupsForDevEUI(ctx context.Context, db sqlx.Queryer, devEUI lorawan.EUI64) ([]uuid.UUID, error) {
	var out []uuid.UUID

	err := sqlx.Select(db, &out, `
		select
			multicast_group_id
		from
			device_multicast_group
		where
			dev_eui = $1`,
		devEUI[:])
	if err != nil {
		return nil, handlePSQLError(err, "select error")
	}

	return out, nil
}

// GetDevEUIsForMulticastGroup returns all Device EUIs within the given
// multicast-group id.
func GetDevEUIsForMulticastGroup(ctx context.Context, db sqlx.Queryer, multicastGroupID uuid.UUID) ([]lorawan.EUI64, error) {
	var out []lorawan.EUI64

	err := sqlx.Select(db, &out, `
		select
			dev_eui
		from
			device_multicast_group
		where
			multicast_group_id = $1
	`, multicastGroupID)
	if err != nil {
		return nil, handlePSQLError(err, "select error")
	}

	return out, nil
}
