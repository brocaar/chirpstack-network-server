package multicast

import (
	"crypto/rand"
	"encoding/binary"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/backend/gateway"
	"github.com/brocaar/loraserver/internal/band"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/framelog"
	"github.com/brocaar/loraserver/internal/helpers"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/lorawan"
)

var errAbort = errors.New("")

type multicastContext struct {
	Token              uint16
	DB                 sqlx.Ext
	MulticastGroup     storage.MulticastGroup
	MulticastQueueItem storage.MulticastQueueItem
	TXInfo             gw.DownlinkTXInfo
	PHYPayload         lorawan.PHYPayload
}

var multicastTasks = []func(*multicastContext) error{
	getMulticastGroup,
	setToken,
	removeQueueItem,
	validatePayloadSize,
	setTXInfo,
	setPHYPayload,
	sendDownlinkData,
}

var (
	downlinkLockDuration time.Duration
	schedulerInterval    time.Duration
	installationMargin   float64
	downlinkTXPower      int

	// TODO: make configurable
	classBEnqueueMargin = time.Second * 5
)

// Setup sets up the multicast package.
func Setup(conf config.Config) error {
	downlinkLockDuration = conf.NetworkServer.Scheduler.ClassC.DownlinkLockDuration
	schedulerInterval = conf.NetworkServer.Scheduler.SchedulerInterval
	installationMargin = conf.NetworkServer.NetworkSettings.InstallationMargin
	downlinkTXPower = conf.NetworkServer.NetworkSettings.DownlinkTXPower

	return nil
}

// HandleScheduleNextQueueItem handles the scheduling of the next queue-item
// for the given multicast-group.
func HandleScheduleNextQueueItem(db sqlx.Ext, mg storage.MulticastGroup) error {
	ctx := multicastContext{
		DB:             db,
		MulticastGroup: mg,
	}

	for _, t := range multicastTasks {
		if err := t(&ctx); err != nil {
			if err == errAbort {
				return nil
			}
			return err
		}
	}

	return nil
}

// HandleScheduleQueueItem handles the scheduling of the given queue-item.
func HandleScheduleQueueItem(db sqlx.Ext, qi storage.MulticastQueueItem) error {
	ctx := multicastContext{
		DB:                 db,
		MulticastQueueItem: qi,
	}

	for _, t := range multicastTasks {
		if err := t(&ctx); err != nil {
			if err == errAbort {
				return nil
			}
			return err
		}
	}

	return nil
}

func getMulticastGroup(ctx *multicastContext) error {
	var err error
	ctx.MulticastGroup, err = storage.GetMulticastGroup(ctx.DB, ctx.MulticastQueueItem.MulticastGroupID, false)
	if err != nil {
		return errors.Wrap(err, "get multicast-group error")
	}

	return nil
}

func setToken(ctx *multicastContext) error {
	b := make([]byte, 2)
	_, err := rand.Read(b)
	if err != nil {
		return errors.Wrap(err, "read random error")
	}
	ctx.Token = binary.BigEndian.Uint16(b)
	return nil
}

func removeQueueItem(ctx *multicastContext) error {
	if err := storage.DeleteMulticastQueueItem(ctx.DB, ctx.MulticastQueueItem.ID); err != nil {
		return errors.Wrap(err, "delete multicast queue-item error")
	}

	return nil
}

func validatePayloadSize(ctx *multicastContext) error {
	maxSize, err := band.Band().GetMaxPayloadSizeForDataRateIndex("", "", ctx.MulticastGroup.DR)
	if err != nil {
		return errors.Wrap(err, "get max payload-size for data-rate index error")
	}

	if len(ctx.MulticastQueueItem.FRMPayload) > maxSize.N {
		log.WithFields(log.Fields{
			"multicast_group_id":   ctx.MulticastGroup.ID,
			"dr":                   ctx.MulticastGroup.DR,
			"max_frm_payload_size": maxSize.N,
			"frm_payload_size":     len(ctx.MulticastQueueItem.FRMPayload),
		}).Error("payload exceeds max size for data-rate")

		return errAbort
	}

	return nil
}

func setTXInfo(ctx *multicastContext) error {
	txInfo := gw.DownlinkTXInfo{
		GatewayId: ctx.MulticastQueueItem.GatewayID[:],
		Frequency: uint32(ctx.MulticastGroup.Frequency),
	}

	if ctx.MulticastQueueItem.EmitAtTimeSinceGPSEpoch == nil {
		txInfo.Timing = gw.DownlinkTiming_IMMEDIATELY
		txInfo.TimingInfo = &gw.DownlinkTXInfo_ImmediatelyTimingInfo{
			ImmediatelyTimingInfo: &gw.ImmediatelyTimingInfo{},
		}
	} else {
		txInfo.Timing = gw.DownlinkTiming_GPS_EPOCH
		txInfo.TimingInfo = &gw.DownlinkTXInfo_GpsEpochTimingInfo{
			GpsEpochTimingInfo: &gw.GPSEpochTimingInfo{
				TimeSinceGpsEpoch: ptypes.DurationProto(*ctx.MulticastQueueItem.EmitAtTimeSinceGPSEpoch),
			},
		}
	}

	if err := helpers.SetDownlinkTXInfoDataRate(&txInfo, ctx.MulticastGroup.DR, band.Band()); err != nil {
		return errors.Wrap(err, "set data-rate error")
	}

	if downlinkTXPower != -1 {
		txInfo.Power = int32(downlinkTXPower)
	} else {
		txInfo.Power = int32(band.Band().GetDownlinkTXPower(ctx.MulticastGroup.Frequency))
	}

	ctx.TXInfo = txInfo

	return nil
}

func setPHYPayload(ctx *multicastContext) error {
	ctx.PHYPayload = lorawan.PHYPayload{
		MHDR: lorawan.MHDR{
			MType: lorawan.UnconfirmedDataDown,
			Major: lorawan.LoRaWANR1,
		},
		MACPayload: &lorawan.MACPayload{
			FHDR: lorawan.FHDR{
				DevAddr: ctx.MulticastGroup.MCAddr,
				FCnt:    ctx.MulticastQueueItem.FCnt,
			},
			FPort:      &ctx.MulticastQueueItem.FPort,
			FRMPayload: []lorawan.Payload{&lorawan.DataPayload{Bytes: ctx.MulticastQueueItem.FRMPayload}},
		},
	}

	// using LoRaWAN1_0 vs LoRaWAN1_1 only makes a difference when setting the
	// confirmed frame-counter
	if err := ctx.PHYPayload.SetDownlinkDataMIC(lorawan.LoRaWAN1_1, 0, ctx.MulticastGroup.MCNwkSKey); err != nil {
		return errors.Wrap(err, "set downlink data mic error")
	}

	return nil
}

func sendDownlinkData(ctx *multicastContext) error {
	phyB, err := ctx.PHYPayload.MarshalBinary()
	if err != nil {
		return errors.Wrap(err, "marshal phypayload error")
	}

	downlinkFrame := gw.DownlinkFrame{
		Token:      uint32(ctx.Token),
		TxInfo:     &ctx.TXInfo,
		PhyPayload: phyB,
	}

	if err := gateway.Backend().SendTXPacket(downlinkFrame); err != nil {
		return errors.Wrap(err, "send downlink frame to gateway error")
	}

	if err := framelog.LogDownlinkFrameForGateway(storage.RedisPool(), downlinkFrame); err != nil {
		log.WithError(err).Error("log downlink frame for gateway error")
	}

	return nil
}
