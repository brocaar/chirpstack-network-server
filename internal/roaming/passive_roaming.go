package roaming

import (
	"context"
	"fmt"
	"time"

	"github.com/gofrs/uuid"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/chirpstack-network-server/internal/band"
	"github.com/brocaar/chirpstack-network-server/internal/logging"
	"github.com/brocaar/chirpstack-network-server/internal/models"
	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/backend"
)

// HandlePassiveRoamingUplink handles the given uplink as passive-roaming.
func HandlePassiveRoamingUplink(ctx context.Context, rxPacket models.RXPacket, macPL *lorawan.MACPayload) {
	// Check if we have already a roaming session for the received PHYPayload.
	roamingSessions, err := storage.GetPassiveRoamingDeviceSessionsForPHYPayload(ctx, rxPacket.PHYPayload)
	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"ctx_id": ctx.Value(logging.ContextIDKey),
		}).Error("roaming: get passive-roaming device-sessions error")
		return
	}

	if len(roamingSessions) == 0 {
		// Start roaming session if there is an agreement.
		// There is a possibility that there are multiple matches.
		for _, a := range agreements {
			if macPL.FHDR.DevAddr.IsNetID(a.netID) && a.passiveRoaming {
				sess, err := prStart(ctx, a.netID, a.client, rxPacket, macPL)
				if err != nil {
					log.WithError(err).WithFields(log.Fields{
						"net_id":   a.netID,
						"dev_addr": macPL.FHDR.DevAddr,
						"ctx_id":   ctx.Value(logging.ContextIDKey),
					}).Error("roaming: start passive-roaming error")
					continue
				}

				if err := storage.SavePassiveRoamingDeviceSession(ctx, &sess); err != nil {
					log.WithError(err).WithFields(log.Fields{
						"ctx_id": ctx.Value(logging.ContextIDKey),
					}).Error("roaming: save passive-roaming session error")
					continue
				}

				// Add the new session to roamingSessions so that we can use it
				// in the loop below to forward data.
				roamingSessions = append(roamingSessions, sess)
			}
		}
	}

	// Forward uplink message for each roaming-session.
	for _, rs := range roamingSessions {
		// Get API client.
		client, err := GetClientForNetID(rs.NetID)
		if err != nil {
			log.WithError(err).WithFields(log.Fields{
				"net_id": rs.NetID,
				"ctx_id": ctx.Value(logging.ContextIDKey),
			}).Error("roaming: get api client for netid error")
			continue
		}

		// forward data
		if err := xmitDataUplink(ctx, client, rxPacket, macPL); err != nil {
			log.WithError(err).WithFields(log.Fields{
				"net_id":                            rs.NetID,
				"dev_addr":                          macPL.FHDR.DevAddr,
				"f_cnt":                             macPL.FHDR.FCnt,
				"passive_roaming_device_session_id": rs.SessionID,
				"ctx_id":                            ctx.Value(logging.ContextIDKey),
			}).Error("roaming: XmitDataReq failed")
			continue
		}

		log.WithFields(log.Fields{
			"net_id":                            rs.NetID,
			"dev_addr":                          macPL.FHDR.DevAddr,
			"f_cnt":                             macPL.FHDR.FCnt,
			"passive_roaming_device_session_id": rs.SessionID,
			"ctx_id":                            ctx.Value(logging.ContextIDKey),
		}).Info("roaming: forwarded uplink using passive-roaming")
	}
}

func xmitDataUplink(ctx context.Context, client backend.Client, rxPacket models.RXPacket, macPL *lorawan.MACPayload) error {
	phyB, err := rxPacket.PHYPayload.MarshalBinary()
	if err != nil {
		return errors.Wrap(err, "marshal phypayload error")
	}

	ulFreq := float64(rxPacket.TXInfo.Frequency) / 1000000
	gwCnt := len(rxPacket.RXInfoSet)

	req := backend.XmitDataReqPayload{
		PHYPayload: backend.HEXBytes(phyB),
		ULMetaData: &backend.ULMetaData{
			DataRate: &rxPacket.DR,
			ULFreq:   &ulFreq,
			RFRegion: band.Band().Name(),
			GWCnt:    &gwCnt,
		},
	}

	for i := range rxPacket.RXInfoSet {
		rxInfo := rxPacket.RXInfoSet[i]
		var lat, lon *float64

		if loc := rxInfo.GetLocation(); loc != nil {
			lat = &loc.Latitude
			lon = &loc.Longitude
		}

		b, err := proto.Marshal(rxInfo)
		if err != nil {
			return errors.Wrap(err, "marshal protobuf error")
		}

		rssi := int(rxInfo.Rssi)
		req.ULMetaData.GWInfo = append(req.ULMetaData.GWInfo, backend.GWInfoElement{
			ID:        backend.HEXBytes(rxInfo.GatewayId),
			RFRegion:  band.Band().Name(),
			RSSI:      &rssi,
			SNR:       &rxInfo.LoraSnr,
			Lat:       lat,
			Lon:       lon,
			DLAllowed: true,
			ULToken:   backend.HEXBytes(b),
		})
	}

	resp, err := client.XmitDataReq(ctx, req)
	if err != nil {
		return errors.Wrap(err, "request error")
	}

	if resp.Result.ResultCode != backend.Success {
		return fmt.Errorf("expected: %s, got: %s (%s)", backend.Success, resp.Result.ResultCode, resp.Result.Description)
	}

	return nil
}

func prStart(ctx context.Context, netID lorawan.NetID, client backend.Client, rxPacket models.RXPacket, macPL *lorawan.MACPayload) (storage.PassiveRoamingDeviceSession, error) {
	var out storage.PassiveRoamingDeviceSession

	phyB, err := rxPacket.PHYPayload.MarshalBinary()
	if err != nil {
		return out, errors.Wrap(err, "marshal phypayload error")
	}

	ulFreq := float64(rxPacket.TXInfo.Frequency) / 1000000
	gwCnt := len(rxPacket.RXInfoSet)

	req := backend.PRStartReqPayload{
		PHYPayload: backend.HEXBytes(phyB),
		ULMetaData: backend.ULMetaData{
			DataRate: &rxPacket.DR,
			ULFreq:   &ulFreq,
			RFRegion: band.Band().Name(),
			GWCnt:    &gwCnt,
		},
	}

	resp, err := client.PRStartReq(ctx, req)
	if err != nil {
		return out, errors.Wrap(err, "request error")
	}

	if resp.Result.ResultCode != backend.Success {
		return out, fmt.Errorf("expected: %s, got: %s (%s)", backend.Success, resp.Result.ResultCode, resp.Result.Description)
	}

	out.NetID = netID
	out.DevAddr = macPL.FHDR.DevAddr
	out.SessionID, err = uuid.NewV4()
	if err != nil {
		return out, errors.Wrap(err, "new uuid error")
	}

	// DevEUI
	if devEUI := resp.DevEUI; devEUI != nil {
		out.DevEUI = *devEUI
	}

	// Lifetime
	out.Lifetime = time.Now()
	if lifetime := resp.Lifetime; lifetime != nil {
		out.Lifetime.Add(time.Duration(*lifetime) * time.Second)
	}

	// FNwkSIntKey (LoRaWAN 1.1)
	if fNwkSIntKey := resp.FNwkSIntKey; fNwkSIntKey != nil {
		out.LoRaWAN11 = true

		if fNwkSIntKey.KEKLabel != "" {
			kek, err := GetKEKKey(fNwkSIntKey.KEKLabel)
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
			kek, err := GetKEKKey(nwkSKey.KEKLabel)
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
