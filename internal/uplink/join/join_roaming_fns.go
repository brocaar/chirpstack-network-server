package join

import (
	"context"
	"time"

	"github.com/gofrs/uuid"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/chirpstack-network-server/internal/backend/joinserver"
	"github.com/brocaar/chirpstack-network-server/internal/band"
	dlroaming "github.com/brocaar/chirpstack-network-server/internal/downlink/roaming"
	"github.com/brocaar/chirpstack-network-server/internal/logging"
	"github.com/brocaar/chirpstack-network-server/internal/models"
	"github.com/brocaar/chirpstack-network-server/internal/roaming"
	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/backend"
)

type startPRFNSContext struct {
	ctx                context.Context
	rxPacket           models.RXPacket
	joinRequestPayload *lorawan.JoinRequestPayload
	homeNetID          lorawan.NetID
	nsClient           backend.Client
	prStartAns         backend.PRStartAnsPayload
}

// StartPRFNS initiates the passive-roaming OTAA as a fNS.
func StartPRFNS(ctx context.Context, rxPacket models.RXPacket, jrPL *lorawan.JoinRequestPayload) error {
	cctx := startPRFNSContext{
		ctx:                ctx,
		rxPacket:           rxPacket,
		joinRequestPayload: jrPL,
	}

	for _, f := range []func() error{
		cctx.getHomeNetID,
		cctx.getNSClient,
		cctx.startRoaming,
		cctx.saveRoamingSession,
	} {
		if err := f(); err != nil {
			return err
		}
	}

	return nil
}

func (ctx *startPRFNSContext) getHomeNetID() error {
	jsClient, err := joinserver.GetClientForJoinEUI(ctx.joinRequestPayload.JoinEUI)
	if err != nil {
		return errors.Wrap(err, "get js client for joineui error")
	}

	nsReq := backend.HomeNSReqPayload{
		DevEUI: ctx.joinRequestPayload.DevEUI,
	}
	nsAns, err := jsClient.HomeNSReq(ctx.ctx, nsReq)
	if err != nil {
		return errors.Wrap(err, "request home netid error")
	}

	log.WithFields(log.Fields{
		"ctx_id":   ctx.ctx.Value(logging.ContextIDKey),
		"net_id":   nsAns.HNetID,
		"join_eui": ctx.joinRequestPayload.JoinEUI,
		"dev_eui":  ctx.joinRequestPayload.DevEUI,
	}).Info("uplink/join: resolved joineui to netid")

	ctx.homeNetID = nsAns.HNetID

	return nil
}

func (ctx *startPRFNSContext) getNSClient() error {
	client, err := roaming.GetClientForNetID(ctx.homeNetID)
	if err != nil {
		if err == roaming.ErrNoAgreement {
			log.WithFields(log.Fields{
				"net_id":  ctx.homeNetID,
				"ctx_id":  ctx.ctx.Value(logging.ContextIDKey),
				"dev_eui": ctx.joinRequestPayload.DevEUI,
			}).Warning("uplink/join: no roaming agreement for netid")
			return ErrAbort
		}
	}

	ctx.nsClient = client
	return nil
}

func (ctx *startPRFNSContext) startRoaming() error {
	phyB, err := ctx.rxPacket.PHYPayload.MarshalBinary()
	if err != nil {
		return errors.Wrap(err, "marshal phypayload error")
	}

	gwCnt := len(ctx.rxPacket.RXInfoSet)
	gwInfo, err := roaming.RXInfoToGWInfo(ctx.rxPacket.RXInfoSet)
	if err != nil {
		return errors.Wrap(err, "rxinfo to gwinfo error")
	}

	ulFreq := float64(ctx.rxPacket.TXInfo.Frequency) / 1000000

	prReq := backend.PRStartReqPayload{
		PHYPayload: backend.HEXBytes(phyB),
		ULMetaData: backend.ULMetaData{
			DevEUI:   &ctx.joinRequestPayload.DevEUI,
			ULFreq:   &ulFreq,
			DataRate: &ctx.rxPacket.DR,
			RecvTime: roaming.RecvTimeFromRXInfo(ctx.rxPacket.RXInfoSet),
			RFRegion: band.Band().Name(),
			GWCnt:    &gwCnt,
			GWInfo:   gwInfo,
		},
	}

	ctx.prStartAns, err = ctx.nsClient.PRStartReq(ctx.ctx, prReq)
	if err != nil {
		return errors.Wrap(err, "PRStartReq error")
	}

	if ctx.prStartAns.DLMetaData == nil {
		return errors.New("DLMetaData must not be nil")
	}

	if err := dlroaming.EmitPRDownlink(ctx.ctx, ctx.rxPacket, ctx.prStartAns.PHYPayload, *ctx.prStartAns.DLMetaData); err != nil {
		return errors.Wrap(err, "send passive-roaming downlink error")
	}

	return nil
}

func (ctx *startPRFNSContext) saveRoamingSession() error {
	if ctx.prStartAns.DevAddr == nil || ctx.prStartAns.Lifetime == nil || *ctx.prStartAns.Lifetime == 0 {
		return nil
	}

	id, err := uuid.NewV4()
	if err != nil {
		return errors.Wrap(err, "get new uuid error")
	}

	sess := storage.PassiveRoamingDeviceSession{
		SessionID: id,
		NetID:     ctx.homeNetID,
		DevEUI:    ctx.joinRequestPayload.DevEUI,
		DevAddr:   *ctx.prStartAns.DevAddr,
		Lifetime:  time.Now().Add(time.Duration(*ctx.prStartAns.Lifetime) * time.Second),
	}

	// FNwkSIntKey (LoRaWAN 1.1)
	if fNwkSIntKey := ctx.prStartAns.FNwkSIntKey; fNwkSIntKey != nil {
		sess.LoRaWAN11 = true

		if fNwkSIntKey.KEKLabel != "" {
			kek, err := roaming.GetKEKKey(fNwkSIntKey.KEKLabel)
			if err != nil {
				return errors.Wrap(err, "get kek error")
			}

			key, err := fNwkSIntKey.Unwrap(kek)
			if err != nil {
				return errors.Wrap(err, "unwrap FNwkSIntKey with kek error")
			}
			sess.FNwkSIntKey = key
		} else {
			copy(sess.FNwkSIntKey[:], fNwkSIntKey.AESKey[:])
		}
	}

	// NwkSKey (LoRaWAN 1.0)
	if nwkSKey := ctx.prStartAns.NwkSKey; nwkSKey != nil {
		if nwkSKey.KEKLabel != "" {
			kek, err := roaming.GetKEKKey(nwkSKey.KEKLabel)
			if err != nil {
				return errors.Wrap(err, "get kek error")
			}

			key, err := nwkSKey.Unwrap(kek)
			if err != nil {
				return errors.Wrap(err, "unwrap NwkSKey with kek error")
			}
			sess.FNwkSIntKey = key
		} else {
			copy(sess.FNwkSIntKey[:], nwkSKey.AESKey[:])
		}
	}

	if err := storage.SavePassiveRoamingDeviceSession(ctx.ctx, &sess); err != nil {
		return errors.Wrap(err, "save passive-roaming device-session error")
	}

	return nil
}
