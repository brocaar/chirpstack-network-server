package storage

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/chirpstack-network-server/internal/gps"
	"github.com/brocaar/chirpstack-network-server/internal/helpers/classb"
	"github.com/brocaar/chirpstack-network-server/internal/logging"
	"github.com/brocaar/lorawan"
)

const (
	classBScheduleMargin = 5 * time.Second
)

// DeviceQueueItem represents an item in the device queue (downlink).
type DeviceQueueItem struct {
	ID                      int64           `db:"id"`
	CreatedAt               time.Time       `db:"created_at"`
	UpdatedAt               time.Time       `db:"updated_at"`
	DevAddr                 lorawan.DevAddr `db:"dev_addr"`
	DevEUI                  lorawan.EUI64   `db:"dev_eui"`
	FRMPayload              []byte          `db:"frm_payload"`
	FCnt                    uint32          `db:"f_cnt"`
	FPort                   uint8           `db:"f_port"`
	Confirmed               bool            `db:"confirmed"`
	IsPending               bool            `db:"is_pending"`
	EmitAtTimeSinceGPSEpoch *time.Duration  `db:"emit_at_time_since_gps_epoch"`
	TimeoutAfter            *time.Time      `db:"timeout_after"`
	RetryAfter              *time.Time      `db:"retry_after"`
}

// Validate validates the DeviceQueueItem.
func (d DeviceQueueItem) Validate() error {
	if d.FPort == 0 {
		return ErrInvalidFPort
	}
	return nil
}

// CreateDeviceQueueItem adds the given item to the device-queue.
// In case the device is operating in Class-B, this will schedule the item
// at the next ping-slot.
func CreateDeviceQueueItem(ctx context.Context, db sqlx.Queryer, qi *DeviceQueueItem, dp DeviceProfile, ds DeviceSession) error {
	if err := qi.Validate(); err != nil {
		return err
	}

	// If the device is operating in Class-B and has a beacon lock, calculate
	// the next ping-slot.
	if qi.TimeoutAfter == nil && qi.EmitAtTimeSinceGPSEpoch == nil && dp.SupportsClassB && ds.BeaconLocked {
		// If there are other Class-B queue-items, we want to calculate the next
		// ping-slot after the last queue item.
		scheduleAfterGPSEpochTS, err := GetMaxEmitAtTimeSinceGPSEpochForDevEUI(ctx, db, qi.DevEUI)
		if err != nil {
			return errors.Wrap(err, "get max emit-at time since gps epoch for deveui error")
		}

		// If no queue-items exist, we start at the current time.
		if scheduleAfterGPSEpochTS == 0 {
			scheduleAfterGPSEpochTS = gps.Time(time.Now()).TimeSinceGPSEpoch()
		}

		// Add some margin.
		scheduleAfterGPSEpochTS += classBScheduleMargin

		// Calculate the next ping-slot after scheduleAfterGPSEpochTS.
		gpsEpochTS, err := classb.GetNextPingSlotAfter(scheduleAfterGPSEpochTS, ds.DevAddr, ds.PingSlotNb)
		if err != nil {
			return errors.Wrap(err, "get next ping-slot error")
		}

		// We expect an ACK after Class-B Timeout.
		timeoutTime := time.Time(gps.NewFromTimeSinceGPSEpoch(gpsEpochTS)).Add(time.Second * time.Duration(dp.ClassBTimeout))
		qi.EmitAtTimeSinceGPSEpoch = &gpsEpochTS
		qi.TimeoutAfter = &timeoutTime
	}

	now := time.Now()
	qi.CreatedAt = now
	qi.UpdatedAt = now

	err := sqlx.Get(db, &qi.ID, `
        insert into device_queue (
            created_at,
            updated_at,
			dev_addr,
            dev_eui,
            frm_payload,
            f_cnt,
            f_port,
            confirmed,
            emit_at_time_since_gps_epoch,
            is_pending,
            timeout_after,
			retry_after
		) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
        returning id`,
		qi.CreatedAt,
		qi.UpdatedAt,
		qi.DevAddr[:],
		qi.DevEUI[:],
		qi.FRMPayload,
		qi.FCnt,
		qi.FPort,
		qi.Confirmed,
		qi.EmitAtTimeSinceGPSEpoch,
		qi.IsPending,
		qi.TimeoutAfter,
		qi.RetryAfter,
	)
	if err != nil {
		return handlePSQLError(err, "insert error")
	}

	log.WithFields(log.Fields{
		"dev_eui": qi.DevEUI,
		"f_cnt":   qi.FCnt,
		"ctx_id":  ctx.Value(logging.ContextIDKey),
	}).Info("device-queue item created")

	return nil
}

// GetDeviceQueueItem returns the device-queue item matching the given id.
func GetDeviceQueueItem(ctx context.Context, db sqlx.Queryer, id int64) (DeviceQueueItem, error) {
	var qi DeviceQueueItem
	err := sqlx.Get(db, &qi, "select * from device_queue where id = $1", id)
	if err != nil {
		return qi, handlePSQLError(err, "select error")
	}
	return qi, nil
}

// UpdateDeviceQueueItem updates the given device-queue item.
func UpdateDeviceQueueItem(ctx context.Context, db sqlx.Execer, qi *DeviceQueueItem) error {
	qi.UpdatedAt = time.Now()

	res, err := db.Exec(`
        update device_queue
        set
            updated_at = $2,
            dev_eui = $3,
            frm_payload = $4,
            f_cnt = $5,
            f_port = $6,
            confirmed = $7,
            emit_at_time_since_gps_epoch = $8,
            is_pending = $9,
            timeout_after = $10,
			dev_addr = $11,
			retry_after = $12
        where
            id = $1`,
		qi.ID,
		qi.UpdatedAt,
		qi.DevEUI[:],
		qi.FRMPayload,
		qi.FCnt,
		qi.FPort,
		qi.Confirmed,
		qi.EmitAtTimeSinceGPSEpoch,
		qi.IsPending,
		qi.TimeoutAfter,
		qi.DevAddr[:],
		qi.RetryAfter,
	)
	if err != nil {
		return handlePSQLError(err, "update error")
	}
	ra, err := res.RowsAffected()
	if err != nil {
		return errors.Wrap(err, "get rows affected error")
	}
	if ra == 0 {
		return ErrDoesNotExist
	}

	log.WithFields(log.Fields{
		"f_cnt":                        qi.FCnt,
		"dev_eui":                      qi.DevEUI,
		"is_pending":                   qi.IsPending,
		"emit_at_time_since_gps_epoch": qi.EmitAtTimeSinceGPSEpoch,
		"timeout_after":                qi.TimeoutAfter,
		"ctx_id":                       ctx.Value(logging.ContextIDKey),
	}).Info("device-queue item updated")

	return nil
}

// DeleteDeviceQueueItem deletes the device-queue item matching the given id.
func DeleteDeviceQueueItem(ctx context.Context, db sqlx.Execer, id int64) error {
	res, err := db.Exec("delete from device_queue where id = $1", id)
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
	}).Info("device-queue deleted")

	return nil
}

// FlushDeviceQueueForDevEUI deletes all device-queue items for the given DevEUI.
func FlushDeviceQueueForDevEUI(ctx context.Context, db sqlx.Execer, devEUI lorawan.EUI64) error {
	_, err := db.Exec("delete from device_queue where dev_eui = $1", devEUI[:])
	if err != nil {
		return handlePSQLError(err, "delete error")
	}

	log.WithFields(log.Fields{
		"ctx_id":  ctx.Value(logging.ContextIDKey),
		"dev_eui": devEUI,
	}).Info("device-queue flushed")

	return nil
}

// GetNextDeviceQueueItemForDevEUI returns the next device-queue item for the
// given DevEUI, ordered by f_cnt and a bool indicating if more items exist in
// the queue (note that the f_cnt should never roll over).
func GetNextDeviceQueueItemForDevEUI(ctx context.Context, db sqlx.Queryer, devEUI lorawan.EUI64) (DeviceQueueItem, bool, error) {
	var items []DeviceQueueItem
	err := sqlx.Select(db, &items, `
        select
            *
        from
            device_queue
        where
            dev_eui = $1
        order by
            f_cnt
        limit 2`,
		devEUI[:],
	)
	if err != nil {
		return DeviceQueueItem{}, false, handlePSQLError(err, "select error")
	}

	if len(items) == 0 {
		return DeviceQueueItem{}, false, ErrDoesNotExist
	}

	// we are interested in the first item
	qi := items[0]

	// In case the transmission is pending and hasn't timed-out yet, do not
	// return it.
	if qi.IsPending && qi.TimeoutAfter != nil && qi.TimeoutAfter.After(time.Now()) {
		return DeviceQueueItem{}, false, ErrDoesNotExist
	}

	return qi, len(items) > 1, nil
}

// GetPendingDeviceQueueItemForDevEUI returns the pending device-queue item for the
// given DevEUI.
func GetPendingDeviceQueueItemForDevEUI(ctx context.Context, db sqlx.Queryer, devEUI lorawan.EUI64) (DeviceQueueItem, error) {
	var qi DeviceQueueItem
	err := sqlx.Get(db, &qi, `
        select
            *
        from
            device_queue
        where
            dev_eui = $1
        order by
            f_cnt
        limit 1`,
		devEUI[:],
	)
	if err != nil {
		return qi, handlePSQLError(err, "select error")
	}

	if !qi.IsPending {
		return qi, ErrDoesNotExist
	}

	return qi, nil
}

// GetDeviceQueueItemsForDevEUI returns all device-queue items for the given
// DevEUI, ordered by id (keep in mind FCnt rollover).
func GetDeviceQueueItemsForDevEUI(ctx context.Context, db sqlx.Queryer, devEUI lorawan.EUI64) ([]DeviceQueueItem, error) {
	var items []DeviceQueueItem
	err := sqlx.Select(db, &items, `
        select
            *
        from
            device_queue
        where
            dev_eui = $1
        order by
            f_cnt`,
		devEUI,
	)
	if err != nil {
		return nil, handlePSQLError(err, "select error")
	}

	return items, nil
}

// GetDeviceQueueItemCountForDevEUI returns the device-queue item count for
// the given DevEUI.
func GetDeviceQueueItemCountForDevEUI(ctx context.Context, db sqlx.Queryer, devEUI lorawan.EUI64) (int, error) {
	var count int
	err := sqlx.Get(db, &count, `
		select
			count(*)
		from
			device_queue
		where
			dev_eui = $1
	`, devEUI)
	if err != nil {
		return 0, handlePSQLError(err, "select error")
	}

	return count, nil
}

// GetDevicesWithClassBOrClassCDeviceQueueItems returns a slice of devices that qualify
// for downlink Class-C transmission.
// The device records will be locked for update so that multiple instances can
// run this query in parallel without the risk of duplicate scheduling.
func GetDevicesWithClassBOrClassCDeviceQueueItems(ctx context.Context, db sqlx.Ext, count int) ([]Device, error) {
	gpsEpochScheduleTime := gps.Time(time.Now().Add(schedulerInterval * 2)).TimeSinceGPSEpoch()

	var devices []Device
	err := sqlx.Select(db, &devices, `
        select
            d.*
        from
            device d
        where
			d.mode in ('B', 'C')
            -- we want devices with queue items
            and exists (
                select
                    1
                from
                    device_queue dq
                where
                    dq.dev_eui = d.dev_eui
                    and (
						d.mode = 'C'
                    	or (
							d.mode = 'B'
                    		and dq.emit_at_time_since_gps_epoch <= $2
                    	)
                    )
            )
			-- exclude device which have one of the following below
            and not exists (
                select
                    1
                from
                    device_queue dq
                where
                    dq.dev_eui = d.dev_eui
					and (
						-- pending queue-item with timeout_after in the future
						(dq.is_pending = true and dq.timeout_after > $3)

						-- or retry_after set to a timestamp in the future
						or (dq.retry_after is not null and dq.retry_after > $3)
					)
            )
        order by
            d.dev_eui
        limit $1
        for update of d skip locked`,
		count,
		gpsEpochScheduleTime,
		time.Now(),
	)
	if err != nil {
		return nil, handlePSQLError(err, "select error")
	}

	return devices, nil
}

// GetMaxEmitAtTimeSinceGPSEpochForDevEUI returns the maximum / last GPS
// epoch scheduling timestamp for the given DevEUI.
func GetMaxEmitAtTimeSinceGPSEpochForDevEUI(ctx context.Context, db sqlx.Queryer, devEUI lorawan.EUI64) (time.Duration, error) {
	var timeSinceGPSEpoch time.Duration
	err := sqlx.Get(db, &timeSinceGPSEpoch, `
		select
			coalesce(max(emit_at_time_since_gps_epoch), 0)
		from
			device_queue
		where
			dev_eui = $1`,
		devEUI,
	)
	if err != nil {
		return 0, handlePSQLError(err, "select error")
	}

	return timeSinceGPSEpoch, nil
}
