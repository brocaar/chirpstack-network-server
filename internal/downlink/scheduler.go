package downlink

import (
	"context"
	"time"

	"github.com/gofrs/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/loraserver/internal/downlink/data"
	"github.com/brocaar/loraserver/internal/downlink/multicast"
	"github.com/brocaar/loraserver/internal/logging"
	"github.com/brocaar/loraserver/internal/storage"
)

// DeviceQueueSchedulerLoop starts an infinit loop calling the scheduler loop for Class-B
// and Class-C sheduling.
func DeviceQueueSchedulerLoop() {
	for {
		ctx := context.Background()
		ctxID, err := uuid.NewV4()
		if err != nil {
			log.WithError(err).Error("get new uuid error")
		}
		ctx = context.WithValue(ctx, logging.ContextIDKey, ctxID)

		log.WithFields(log.Fields{
			"ctx_id": ctxID,
		}).Debug("running class-b / class-c scheduler batch")

		if err := ScheduleDeviceQueueBatch(ctx, schedulerBatchSize); err != nil {
			log.WithFields(log.Fields{
				"ctx_id": ctxID,
			}).WithError(err).Error("class-b / class-c scheduler error")
		}
		time.Sleep(schedulerInterval)
	}
}

// MulticastQueueSchedulerLoop starts an infinit loop calling the multicast
// scheduler loop.
func MulticastQueueSchedulerLoop() {
	for {
		ctx := context.Background()
		ctxID, err := uuid.NewV4()
		if err != nil {
			log.WithError(err).Error("get new uuid error")
		}
		ctx = context.WithValue(ctx, logging.ContextIDKey, ctxID)

		log.WithFields(log.Fields{
			"ctx_id": ctxID,
		}).Debug("running multicast scheduler batch")

		if err := ScheduleMulticastQueueBatch(ctx, schedulerBatchSize); err != nil {
			log.WithFields(log.Fields{
				"ctx_id": ctxID,
			}).WithError(err).Error("multicast scheduler error")
		}
		time.Sleep(schedulerInterval)
	}
}

// ScheduleDeviceQueueBatch schedules a downlink batch (Class-B or Class-C).
func ScheduleDeviceQueueBatch(ctx context.Context, size int) error {
	return storage.Transaction(func(tx sqlx.Ext) error {
		devices, err := storage.GetDevicesWithClassBOrClassCDeviceQueueItems(ctx, tx, size)
		if err != nil {
			return errors.Wrap(err, "get deveuis with class-c device-queue items error")
		}

		for _, d := range devices {
			ds, err := storage.GetDeviceSession(ctx, storage.RedisPool(), d.DevEUI)
			if err != nil {
				log.WithError(err).WithFields(log.Fields{
					"dev_eui": d.DevEUI,
					"ctx_id":  ctx.Value(logging.ContextIDKey),
				}).Error("get device-session error")
				continue
			}

			err = data.HandleScheduleNextQueueItem(ctx, ds, d.Mode)
			if err != nil {
				log.WithError(err).WithFields(log.Fields{
					"dev_eui": d.DevEUI,
					"ctx_id":  ctx.Value(logging.ContextIDKey),
				}).Error("schedule next device-queue item error")
			}
		}

		return nil
	})
}

// ScheduleMulticastQueueBatch schedules a donwlink multicast batch (Class-B & -C).
func ScheduleMulticastQueueBatch(ctx context.Context, size int) error {
	return storage.Transaction(func(tx sqlx.Ext) error {
		// this locks the selected queue-items so that this query can be
		// executed by other instances in parallel.
		multicastQueueItems, err := storage.GetSchedulableMulticastQueueItems(ctx, tx, size)
		if err != nil {
			return errors.Wrap(err, "get multicast queue-items error")
		}

		for _, qi := range multicastQueueItems {
			err := multicast.HandleScheduleQueueItem(ctx, tx, qi)
			if err != nil {
				log.WithFields(log.Fields{
					"multicast_group_id": qi.MulticastGroupID,
					"id":                 qi.ID,
					"ctx_id":             ctx.Value(logging.ContextIDKey),
				}).WithError(err).Error("schedule multicast queue-item error")
			}
		}

		return nil
	})
}
