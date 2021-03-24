package multicast

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"

	"github.com/brocaar/chirpstack-network-server/internal/gps"
	"github.com/brocaar/chirpstack-network-server/internal/helpers/classb"
	"github.com/brocaar/chirpstack-network-server/internal/storage"
)

// EnqueueQueueItem selects the gateways that must be used to cover all devices
// within the multicast-group and creates a queue-item for each individial
// gateway.
// Note that an enqueue action increments the frame-counter of the multicast-group.
func EnqueueQueueItem(ctx context.Context, db sqlx.Ext, qi storage.MulticastQueueItem) error {
	// Get multicast-group and lock it.
	mg, err := storage.GetMulticastGroup(ctx, db, qi.MulticastGroupID, true)
	if err != nil {
		return errors.Wrap(err, "get multicast-group error")
	}

	if qi.FCnt < mg.FCnt {
		return ErrInvalidFCnt
	}

	mg.FCnt = qi.FCnt + 1
	if err := storage.UpdateMulticastGroup(ctx, db, &mg); err != nil {
		return errors.Wrap(err, "update multicast-group error")
	}

	// get DevEUIs within the multicast-group.
	devEUIs, err := storage.GetDevEUIsForMulticastGroup(ctx, db, qi.MulticastGroupID)
	if err != nil {
		return errors.Wrap(err, "get deveuis for multicast-group error")
	}

	rxInfoSets, err := storage.GetDeviceGatewayRXInfoSetForDevEUIs(ctx, devEUIs)
	if err != nil {
		return errors.Wrap(err, "get device gateway rx-info set for deveuis errors")
	}

	gatewayIDs, err := GetMinimumGatewaySet(rxInfoSets)
	if err != nil {
		return errors.Wrap(err, "get minimum gateway set error")
	}

	// for each gateway we increment the schedule_at timestamp with one second
	// to avoid colissions.
	if mg.GroupType == storage.MulticastGroupC {
		ts, err := storage.GetMaxScheduleAtForMulticastGroup(ctx, db, mg.ID)
		if err != nil {
			return errors.Wrap(err, "get maximum schedule at error")
		}

		if ts.IsZero() {
			ts = time.Now()
		}

		for _, gatewayID := range gatewayIDs {
			ts = ts.Add(multicastGatewayDelay)
			qi.GatewayID = gatewayID
			qi.ScheduleAt = ts
			if err = storage.CreateMulticastQueueItem(ctx, db, &qi); err != nil {
				return errors.Wrap(err, "create multicast queue-item error")
			}
		}
	}

	// for each gateway the use the next ping-slot
	if mg.GroupType == storage.MulticastGroupB {
		var pingSlotNb int
		if mg.PingSlotPeriod != 0 {
			pingSlotNb = (1 << 12) / mg.PingSlotPeriod
		}

		scheduleTS, err := storage.GetMaxEmitAtTimeSinceGPSEpochForMulticastGroup(ctx, db, mg.ID)
		if err != nil {
			return errors.Wrap(err, "get maximum emit at time since gps epoch error")
		}

		if scheduleTS == 0 {
			scheduleTS = gps.Time(time.Now().Add(classBEnqueueMargin)).TimeSinceGPSEpoch()
		}

		for _, gatewayID := range gatewayIDs {
			scheduleTS, err = classb.GetNextPingSlotAfter(scheduleTS, mg.MCAddr, pingSlotNb)
			if err != nil {
				return errors.Wrap(err, "get next ping-slot after error")
			}

			qi.EmitAtTimeSinceGPSEpoch = &scheduleTS
			qi.ScheduleAt = time.Time(gps.NewFromTimeSinceGPSEpoch(scheduleTS)).Add(-2 * schedulerInterval)
			qi.GatewayID = gatewayID

			if err = storage.CreateMulticastQueueItem(ctx, db, &qi); err != nil {
				return errors.Wrap(err, "create multicast queue-item error")
			}
		}
	}

	return nil
}
