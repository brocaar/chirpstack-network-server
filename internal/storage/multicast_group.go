package storage

import (
	"context"
	"time"

	"github.com/gofrs/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/loraserver/internal/logging"
	"github.com/brocaar/lorawan"
)

// MulticastGroupType type defines the multicast-group type.
type MulticastGroupType string

// Possible multicast-group types.
const (
	MulticastGroupB MulticastGroupType = "B"
	MulticastGroupC MulticastGroupType = "C"
)

// MulticastGroup defines a multicast-group.
type MulticastGroup struct {
	ID               uuid.UUID          `db:"id"`
	CreatedAt        time.Time          `db:"created_at"`
	UpdatedAt        time.Time          `db:"updated_at"`
	MCAddr           lorawan.DevAddr    `db:"mc_addr"`
	MCNwkSKey        lorawan.AES128Key  `db:"mc_nwk_s_key"`
	FCnt             uint32             `db:"f_cnt"`
	GroupType        MulticastGroupType `db:"group_type"`
	DR               int                `db:"dr"`
	Frequency        int                `db:"frequency"`
	PingSlotPeriod   int                `db:"ping_slot_period"`
	RoutingProfileID uuid.UUID          `db:"routing_profile_id"` // there is no downlink data, but it can be used for future error reporting
	ServiceProfileID uuid.UUID          `db:"service_profile_id"`
}

// MulticastQueueItem defines a multicast queue-item.
type MulticastQueueItem struct {
	ID                      int64          `db:"id"`
	CreatedAt               time.Time      `db:"created_at"`
	ScheduleAt              time.Time      `db:"schedule_at"`
	EmitAtTimeSinceGPSEpoch *time.Duration `db:"emit_at_time_since_gps_epoch"`
	MulticastGroupID        uuid.UUID      `db:"multicast_group_id"`
	GatewayID               lorawan.EUI64  `db:"gateway_id"`
	FCnt                    uint32         `db:"f_cnt"`
	FPort                   uint8          `db:"f_port"`
	FRMPayload              []byte         `db:"frm_payload"`
}

// Validate validates the MulticastQueueItem.
func (m MulticastQueueItem) Validate() error {
	if m.FPort == 0 {
		return ErrInvalidFPort
	}
	return nil
}

// CreateMulticastGroup creates the given multi-cast group.
func CreateMulticastGroup(ctx context.Context, db sqlx.Execer, mg *MulticastGroup) error {
	now := time.Now()
	mg.CreatedAt = now
	mg.UpdatedAt = now

	if mg.ID == uuid.Nil {
		var err error
		mg.ID, err = uuid.NewV4()
		if err != nil {
			return errors.Wrap(err, "new uuid v4 error")
		}
	}

	_, err := db.Exec(`
		insert into multicast_group (
			id,
			created_at,
			updated_at,
			mc_addr,
			mc_nwk_s_key,
			f_cnt,
			group_type,
			dr,
			frequency,
			ping_slot_period,
			service_profile_id,
			routing_profile_id
		) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		mg.ID,
		mg.CreatedAt,
		mg.UpdatedAt,
		mg.MCAddr[:],
		mg.MCNwkSKey[:],
		mg.FCnt,
		mg.GroupType,
		mg.DR,
		mg.Frequency,
		mg.PingSlotPeriod,
		mg.ServiceProfileID,
		mg.RoutingProfileID,
	)
	if err != nil {
		return handlePSQLError(err, "insert error")
	}

	log.WithFields(log.Fields{
		"id":     mg.ID,
		"ctx_id": ctx.Value(logging.ContextIDKey),
	}).Info("multicast-group created")

	return nil
}

// GetMulticastGroup returns the multicast-group for the given ID.
func GetMulticastGroup(ctx context.Context, db sqlx.Queryer, id uuid.UUID, forUpdate bool) (MulticastGroup, error) {
	var mg MulticastGroup
	var fu string

	if forUpdate {
		fu = " for update"
	}

	err := sqlx.Get(db, &mg, `
		select
			*
		from
			multicast_group
		where
			id = $1`+fu,
		id,
	)
	if err != nil {
		return mg, handlePSQLError(err, "select error")
	}
	return mg, nil
}

// UpdateMulticastGroup updates the given multicast-grup.
func UpdateMulticastGroup(ctx context.Context, db sqlx.Execer, mg *MulticastGroup) error {
	mg.UpdatedAt = time.Now()

	res, err := db.Exec(`
		update
			multicast_group
		set
			updated_at = $2,
			mc_addr = $3,
			mc_nwk_s_key = $4,
			f_cnt = $5,
			group_type = $6,
			dr = $7,
			frequency = $8,
			ping_slot_period = $9,
			service_profile_id = $10,
			routing_profile_id = $11
		where
			id = $1`,
		mg.ID,
		mg.UpdatedAt,
		mg.MCAddr[:],
		mg.MCNwkSKey[:],
		mg.FCnt,
		mg.GroupType,
		mg.DR,
		mg.Frequency,
		mg.PingSlotPeriod,
		mg.ServiceProfileID,
		mg.RoutingProfileID,
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
		"id":     mg.ID,
		"ctx_id": ctx.Value(logging.ContextIDKey),
	}).Info("multicast-group updated")

	return nil
}

// DeleteMulticastGroup deletes the multicast-group matching the given ID.
func DeleteMulticastGroup(ctx context.Context, db sqlx.Execer, id uuid.UUID) error {
	res, err := db.Exec(`
		delete from
			multicast_group
		where
			id = $1`,
		id,
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
		"id":     id,
		"ctx_id": ctx.Value(logging.ContextIDKey),
	}).Info("multicast-group deleted")

	return nil
}

// CreateMulticastQueueItem adds the given item to the queue.
func CreateMulticastQueueItem(ctx context.Context, db sqlx.Queryer, qi *MulticastQueueItem) error {
	if err := qi.Validate(); err != nil {
		return err
	}

	qi.CreatedAt = time.Now()

	err := sqlx.Get(db, &qi.ID, `
		insert into multicast_queue (
			created_at,
			schedule_at,
			emit_at_time_since_gps_epoch,
			multicast_group_id,
			gateway_id,
			f_cnt,
			f_port,
			frm_payload
		) values ($1, $2, $3, $4, $5, $6, $7, $8)
		returning
			id
		`,
		qi.CreatedAt,
		qi.ScheduleAt,
		qi.EmitAtTimeSinceGPSEpoch,
		qi.MulticastGroupID,
		qi.GatewayID,
		qi.FCnt,
		qi.FPort,
		qi.FRMPayload,
	)
	if err != nil {
		return handlePSQLError(err, "insert error")
	}

	log.WithFields(log.Fields{
		"id":                 qi.ID,
		"f_cnt":              qi.FCnt,
		"gateway_id":         qi.GatewayID,
		"multicast_group_id": qi.MulticastGroupID,
		"ctx_id":             ctx.Value(logging.ContextIDKey),
	}).Info("multicast queue-item created")

	return nil
}

// DeleteMulticastQueueItem deletes the queue-item given an id.
func DeleteMulticastQueueItem(ctx context.Context, db sqlx.Execer, id int64) error {
	res, err := db.Exec(`
		delete from
			multicast_queue
		where
			id = $1
	`, id)
	if err != nil {
		return handlePSQLError(err, "delete error")
	}
	ra, err := res.RowsAffected()
	if err != nil {
		return errors.Wrap(err, "get rows affected error")
	}
	if ra == 0 {
		return ErrDoesNotExist
	}

	log.WithFields(log.Fields{
		"id":     id,
		"ctx_id": ctx.Value(logging.ContextIDKey),
	}).Info("multicast queue-item deleted")

	return nil
}

// FlushMulticastQueueForMulticastGroup flushes the multicast-queue given
// a multicast-group id.
func FlushMulticastQueueForMulticastGroup(ctx context.Context, db sqlx.Execer, multicastGroupID uuid.UUID) error {
	_, err := db.Exec(`
		delete from
			multicast_queue
		where
			multicast_group_id = $1
	`, multicastGroupID)
	if err != nil {
		return handlePSQLError(err, "delete error")
	}

	log.WithFields(log.Fields{
		"multicast_group_id": multicastGroupID,
		"ctx_id":             ctx.Value(logging.ContextIDKey),
	}).Info("multicast-group queue flushed")

	return nil
}

// GetMulticastQueueItemsForMulticastGroup returns all queue-items given
// a multicast-group id.
func GetMulticastQueueItemsForMulticastGroup(ctx context.Context, db sqlx.Queryer, multicastGroupID uuid.UUID) ([]MulticastQueueItem, error) {
	var items []MulticastQueueItem

	err := sqlx.Select(db, &items, `
		select
			*
		from
			multicast_queue
		where
			multicast_group_id = $1
		order by
			id
	`, multicastGroupID)
	if err != nil {
		return nil, handlePSQLError(err, "select error")
	}

	return items, nil
}

// GetSchedulableMulticastQueueItems returns a slice of multicast-queue items
// for scheduling.
// The returned queue-items will be locked for update so that this query can
// be executed in parallel.
func GetSchedulableMulticastQueueItems(ctx context.Context, db sqlx.Ext, count int) ([]MulticastQueueItem, error) {
	var items []MulticastQueueItem
	err := sqlx.Select(db, &items, `
		select
			*
		from
			multicast_queue
		where
			schedule_at <= $2
		order by
			id
		limit $1
		for update skip locked
	`, count, time.Now())
	if err != nil {
		return nil, handlePSQLError(err, "select error")
	}

	return items, nil
}

// GetMaxEmitAtTimeSinceGPSEpochForMulticastGroup returns the maximum / last GPS
// epoch scheduling timestamp for the given multicast-group.
func GetMaxEmitAtTimeSinceGPSEpochForMulticastGroup(ctx context.Context, db sqlx.Queryer, multicastGroupID uuid.UUID) (time.Duration, error) {
	var timeSinceGPSEpoch time.Duration
	err := sqlx.Get(db, &timeSinceGPSEpoch, `
		select
			coalesce(max(emit_at_time_since_gps_epoch), 0)
		from
			multicast_queue
		where
			multicast_group_id = $1
	`, multicastGroupID)
	if err != nil {
		return 0, handlePSQLError(err, "select error")
	}

	return timeSinceGPSEpoch, nil
}

// GetMaxScheduleAtForMulticastGroup returns the maximum schedule at timestamp
// for the given multicast-group.
func GetMaxScheduleAtForMulticastGroup(ctx context.Context, db sqlx.Queryer, multicastGroupID uuid.UUID) (time.Time, error) {
	ts := new(time.Time)

	err := sqlx.Get(db, &ts, `
		select
			max(schedule_at)
		from
			multicast_queue
		where
			multicast_group_id = $1
	`, multicastGroupID)
	if err != nil {
		return time.Time{}, handlePSQLError(err, "select error")
	}

	if ts != nil {
		return *ts, nil
	}
	return time.Time{}, nil
}
