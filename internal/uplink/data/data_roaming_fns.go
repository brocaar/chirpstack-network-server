package data

import (
	"context"
	"fmt"
	"time"

	"github.com/gofrs/uuid"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/chirpstack-network-server/internal/band"
	"github.com/brocaar/chirpstack-network-server/internal/logging"
	"github.com/brocaar/chirpstack-network-server/internal/models"
	"github.com/brocaar/chirpstack-network-server/internal/roaming"
	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/backend"
)

type roamingDataContext struct {
	ctx        context.Context
	rxPacket   models.RXPacket
	macPayload *lorawan.MACPayload

	prDeviceSessions []storage.PassiveRoamingDeviceSession
}

// HandleRoamingFNS handles an uplink as a fNS.
func HandleRoamingFNS(ctx context.Context, rxPacket models.RXPacket, macPL *lorawan.MACPayload) error {
	cctx := roamingDataContext{
		ctx:        ctx,
		rxPacket:   rxPacket,
		macPayload: macPL,
	}

	for _, f := range []func() error{
		cctx.getPassiveRoamingDeviceSessions,
		cctx.startPassiveRoamingSessions,
		cctx.forwardUplinkMessageForSessions,
		cctx.saveSessions,
	} {
		if err := f(); err != nil {
			return err
		}
	}

	return nil
}

func (ctx *roamingDataContext) getPassiveRoamingDeviceSessions() error {
	var err error
	ctx.prDeviceSessions, err = storage.GetPassiveRoamingDeviceSessionsForPHYPayload(ctx.ctx, ctx.rxPacket.PHYPayload)
	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"ctx_id": ctx.ctx.Value(logging.ContextIDKey),
		}).Error("uplink/data: get passive-roaming device-sessions error")
	}

	return nil
}

func (ctx *roamingDataContext) startPassiveRoamingSessions() error {
	// skip this step when we already have active sessions
	if len(ctx.prDeviceSessions) != 0 {
		return nil
	}

	log.WithFields(log.Fields{
		"ctx_id":   ctx.ctx.Value(logging.ContextIDKey),
		"dev_addr": ctx.macPayload.FHDR.DevAddr,
	}).Info("uplink/data: starting passive-roaming sessions with matching netids")

	netIDMatches := roaming.GetNetIDsForDevAddr(ctx.macPayload.FHDR.DevAddr)
	for _, netID := range netIDMatches {
		ds, err := ctx.startPassiveRoamingSession(netID)
		if err != nil {
			log.WithError(err).WithFields(log.Fields{
				"net_id":   netID,
				"dev_addr": ctx.macPayload.FHDR.DevAddr,
				"ctx_id":   ctx.ctx.Value(logging.ContextIDKey),
			}).Error("uplink/data: start passive-roaming error")
			continue
		}

		// No need to store the device-session or call XmitDataReq when
		// lifetime is not set (stateless passive-roaming).
		var nullTime time.Time
		if ds.Lifetime != nullTime {
			ctx.prDeviceSessions = append(ctx.prDeviceSessions, ds)
		}
	}

	return nil
}

func (ctx *roamingDataContext) forwardUplinkMessageForSessions() error {
	for _, ds := range ctx.prDeviceSessions {
		// get client
		client, err := roaming.GetClientForNetID(ds.NetID)
		if err != nil {
			log.WithError(err).WithFields(log.Fields{
				"net_id":   ds.NetID,
				"dev_addr": ctx.macPayload.FHDR.DevAddr,
				"ctx_id":   ctx.ctx.Value(logging.ContextIDKey),
			}).Error("uplink/data: get backend client error")
		}

		// forward data
		if err := ctx.xmitDataUplink(client); err != nil {
			log.WithError(err).WithFields(log.Fields{
				"net_id":                            ds.NetID,
				"dev_addr":                          ctx.macPayload.FHDR.DevAddr,
				"f_cnt":                             ctx.macPayload.FHDR.FCnt,
				"passive_roaming_device_session_id": ds.SessionID,
				"ctx_id":                            ctx.ctx.Value(logging.ContextIDKey),
			}).Error("uplink/data: passive-roaming XmitDataReq failed")
			continue
		}

		log.WithFields(log.Fields{
			"dev_addr":                          ctx.macPayload.FHDR.DevAddr,
			"net_id":                            ds.NetID,
			"f_cnt":                             ctx.macPayload.FHDR.FCnt,
			"passive_roaming_device_session_id": ds.SessionID,
			"ctx_id":                            ctx.ctx.Value(logging.ContextIDKey),
		}).Info("uplink/data: forwarded uplink using passive-roaming")
	}

	return nil
}

func (ctx *roamingDataContext) saveSessions() error {
	for _, ds := range ctx.prDeviceSessions {
		ds.FCntUp = ctx.macPayload.FHDR.FCnt + 1

		if err := storage.SavePassiveRoamingDeviceSession(ctx.ctx, &ds); err != nil {
			log.WithError(err).WithFields(log.Fields{
				"passive_roaming_device_session_id": ds.SessionID,
				"ctx_id":                            ctx.ctx.Value(logging.ContextIDKey),
				"dev_eui":                           ds.DevEUI,
			}).Error("uplink/data: save passive-roaming device-session error")
		}
	}

	return nil
}

func (ctx *roamingDataContext) startPassiveRoamingSession(netID lorawan.NetID) (storage.PassiveRoamingDeviceSession, error) {
	var out storage.PassiveRoamingDeviceSession

	client, err := roaming.GetClientForNetID(netID)
	if err != nil {
		return out, errors.Wrap(err, "get backend client error")
	}

	phyB, err := ctx.rxPacket.PHYPayload.MarshalBinary()
	if err != nil {
		return out, errors.Wrap(err, "marshal phypayload error")
	}

	ulFreq := float64(ctx.rxPacket.TXInfo.Frequency) / 1000000
	gwCnt := len(ctx.rxPacket.RXInfoSet)
	gwInfo, err := roaming.RXInfoToGWInfo(ctx.rxPacket.RXInfoSet)
	if err != nil {
		return out, errors.Wrap(err, "rxinfo to gwinfo error")
	}

	req := backend.PRStartReqPayload{
		PHYPayload: backend.HEXBytes(phyB),
		ULMetaData: backend.ULMetaData{
			DataRate: &ctx.rxPacket.DR,
			ULFreq:   &ulFreq,
			RecvTime: roaming.RecvTimeFromRXInfo(ctx.rxPacket.RXInfoSet),
			RFRegion: band.Band().Name(),
			GWCnt:    &gwCnt,
			GWInfo:   gwInfo,
		},
	}

	log.WithFields(log.Fields{
		"ctx_id":   ctx.ctx.Value(logging.ContextIDKey),
		"dev_addr": ctx.macPayload.FHDR.DevAddr,
		"net_id":   netID,
	}).Info("uplink/data: starting passive-roaming session")

	resp, err := client.PRStartReq(ctx.ctx, req)
	if err != nil {
		return out, errors.Wrap(err, "request error")
	}

	if resp.Result.ResultCode != backend.Success {
		return out, fmt.Errorf("expected: %s, got: %s (%s)", backend.Success, resp.Result.ResultCode, resp.Result.Description)
	}

	out.NetID = netID
	out.DevAddr = ctx.macPayload.FHDR.DevAddr
	out.SessionID, err = uuid.NewV4()
	if err != nil {
		return out, errors.Wrap(err, "new uuid error")
	}

	// DevEUI
	if devEUI := resp.DevEUI; devEUI != nil {
		out.DevEUI = *devEUI
	}

	// Lifetime
	if lifetime := resp.Lifetime; lifetime != nil && *lifetime != 0 {
		out.Lifetime = time.Now().Add(time.Duration(*lifetime) * time.Second)
	}

	// FNwkSIntKey (LoRaWAN 1.1)
	if fNwkSIntKey := resp.FNwkSIntKey; fNwkSIntKey != nil {
		out.LoRaWAN11 = true

		if fNwkSIntKey.KEKLabel != "" {
			kek, err := roaming.GetKEKKey(fNwkSIntKey.KEKLabel)
			if err != nil {
				return out, err
			}

			key, err := fNwkSIntKey.Unwrap(kek)
			if err != nil {
				return out, errors.Wrap(err, "key unwrap error")
			}
			out.FNwkSIntKey = key
		} else {
			copy(out.FNwkSIntKey[:], fNwkSIntKey.AESKey[:])
		}
	}

	// NwkSKey (LoRaWAN 1.0)
	if nwkSKey := resp.NwkSKey; nwkSKey != nil {
		if nwkSKey.KEKLabel != "" {
			kek, err := roaming.GetKEKKey(nwkSKey.KEKLabel)
			if err != nil {
				return out, err
			}

			key, err := nwkSKey.Unwrap(kek)
			if err != nil {
				return out, errors.Wrap(err, "key unwrap error")
			}
			out.FNwkSIntKey = key
		} else {
			copy(out.FNwkSIntKey[:], nwkSKey.AESKey[:])
		}
	}

	// FCntUp
	if fCntUp := resp.FCntUp; fCntUp != nil {
		out.FCntUp = *fCntUp
	}

	return out, nil
}

func (ctx *roamingDataContext) xmitDataUplink(client backend.Client) error {
	phyB, err := ctx.rxPacket.PHYPayload.MarshalBinary()
	if err != nil {
		return errors.Wrap(err, "marshal phypayload error")
	}

	ulFreq := float64(ctx.rxPacket.TXInfo.Frequency) / 1000000
	gwCnt := len(ctx.rxPacket.RXInfoSet)

	req := backend.XmitDataReqPayload{
		PHYPayload: backend.HEXBytes(phyB),
		ULMetaData: &backend.ULMetaData{
			DevAddr:  &ctx.macPayload.FHDR.DevAddr,
			DataRate: &ctx.rxPacket.DR,
			ULFreq:   &ulFreq,
			RecvTime: roaming.RecvTimeFromRXInfo(ctx.rxPacket.RXInfoSet),
			RFRegion: band.Band().Name(),
			GWCnt:    &gwCnt,
		},
	}

	gwInfo, err := roaming.RXInfoToGWInfo(ctx.rxPacket.RXInfoSet)
	if err != nil {
		return errors.Wrap(err, "rxinfo to gwinfo error")
	}
	req.ULMetaData.GWInfo = gwInfo

	resp, err := client.XmitDataReq(ctx.ctx, req)
	if err != nil {
		return errors.Wrap(err, "request error")
	}

	if resp.Result.ResultCode != backend.Success {
		return fmt.Errorf("expected: %s, got: %s (%s)", backend.Success, resp.Result.ResultCode, resp.Result.Description)
	}

	return nil
}
