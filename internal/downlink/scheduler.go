package downlink

import (
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/downlink/data"
	"github.com/brocaar/loraserver/internal/downlink/multicast"
	"github.com/brocaar/loraserver/internal/storage"
)

// DeviceQueueSchedulerLoop starts an infinit loop calling the scheduler loop for Class-B
// and Class-C sheduling.
func DeviceQueueSchedulerLoop() {
	for {
		log.Debug("running class-b / class-c scheduler batch")
		if err := ScheduleDeviceQueueBatch(config.SchedulerBatchSize); err != nil {
			log.WithError(err).Error("class-b / class-c scheduler error")
		}
		time.Sleep(config.C.NetworkServer.Scheduler.SchedulerInterval)
	}
}

// MulticastQueueSchedulerLoop starts an infinit loop calling the multicast
// scheduler loop.
func MulticastQueueSchedulerLoop() {
	for {
		log.Debug("running multicast scheduler batch")
		if err := ScheduleMulticastQueueBatch(config.SchedulerBatchSize); err != nil {
			log.WithError(err).Error("multicast scheduler error")
		}
		time.Sleep(config.C.NetworkServer.Scheduler.SchedulerInterval)
	}
}

// ScheduleDeviceQueueBatch schedules a downlink batch (Class-B or Class-C).
func ScheduleDeviceQueueBatch(size int) error {
	return storage.Transaction(config.C.PostgreSQL.DB, func(tx sqlx.Ext) error {
		devices, err := storage.GetDevicesWithClassBOrClassCDeviceQueueItems(tx, size)
		if err != nil {
			return errors.Wrap(err, "get deveuis with class-c device-queue items error")
		}

		for _, d := range devices {
			ds, err := storage.GetDeviceSession(config.C.Redis.Pool, d.DevEUI)
			if err != nil {
				log.WithError(err).WithField("dev_eui", d.DevEUI).Error("get device-session error")
				continue
			}

			err = data.HandleScheduleNextQueueItem(ds)
			if err != nil {
				log.WithError(err).WithField("dev_eui", d.DevEUI).Error("schedule next device-queue item error")
			}
		}

		return nil
	})
}

// ScheduleMulticastQueueBatch schedules a donwlink multicast batch (Class-B & -C).
func ScheduleMulticastQueueBatch(size int) error {
	return storage.Transaction(config.C.PostgreSQL.DB, func(tx sqlx.Ext) error {
		// this locks the selected queue-items so that this query can be
		// executed by other instances in parallel.
		multicastQueueItems, err := storage.GetSchedulableMulticastQueueItems(tx, size)
		if err != nil {
			return errors.Wrap(err, "get multicast queue-items error")
		}

		for _, qi := range multicastQueueItems {
			err := multicast.HandleScheduleQueueItem(tx, qi)
			if err != nil {
				log.WithFields(log.Fields{
					"multicast_group_id": qi.MulticastGroupID,
					"id":                 qi.ID,
				}).WithError(err).Error("schedule multicast queue-item error")
			}
		}

		return nil
	})
}
