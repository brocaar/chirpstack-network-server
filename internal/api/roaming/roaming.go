package roaming

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/Azure/azure-amqp-common-go/v2/uuid"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/chirpstack-network-server/internal/band"
	"github.com/brocaar/chirpstack-network-server/internal/config"
	"github.com/brocaar/chirpstack-network-server/internal/logging"
	"github.com/brocaar/chirpstack-network-server/internal/models"
	"github.com/brocaar/chirpstack-network-server/internal/roaming"
	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/brocaar/chirpstack-network-server/internal/uplink/data"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/backend"
)

// Setup configures the roaming API.
func Setup(c config.Config) error {
	roamingConfig := c.Roaming

	if roamingConfig.API.Bind == "" {
		log.Debug("api/roaming: roaming is disabled")

		return nil
	}

	log.WithFields(log.Fields{
		"bind":     roamingConfig.API.Bind,
		"ca_cert":  roamingConfig.API.CACert,
		"tls_cert": roamingConfig.API.TLSCert,
		"tls_key":  roamingConfig.API.TLSKey,
	}).Info("api/roaming: starting roaming api")

	server := http.Server{
		Handler:   &API{},
		Addr:      roamingConfig.API.Bind,
		TLSConfig: &tls.Config{},
	}

	if roamingConfig.API.CACert == "" && roamingConfig.API.TLSCert == "" && roamingConfig.API.TLSKey == "" {
		go func() {
			err := server.ListenAndServe()
			log.WithError(err).Fatal("api/roaming: api server error")
		}()
		return nil
	}

	if roamingConfig.API.CACert != "" {
		caCert, err := ioutil.ReadFile(roamingConfig.API.CACert)
		if err != nil {
			return errors.Wrap(err, "read ca certificate error")
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return errors.New("append ca certificate error")
		}

		server.TLSConfig.ClientCAs = caCertPool
		server.TLSConfig.ClientAuth = tls.RequireAndVerifyClientCert

		log.WithFields(log.Fields{
			"ca_cert": roamingConfig.API.CACert,
		}).Info("api/roaming: roaming api is configured with client-certificate authentication")
	}

	go func() {
		err := server.ListenAndServeTLS(roamingConfig.API.TLSCert, roamingConfig.API.TLSKey)
		log.WithError(err).Fatal("api/roaming: api server error")
	}()

	return nil
}

// API implements the roaming API.
type API struct {
	netID lorawan.NetID
}

// ServeHTTP handles a HTTP request.
func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// read request body
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.WithError(err).Error("api/roaming: read request body error")
		return
	}

	// decode the BasePayload part of the request
	var basePL backend.BasePayload
	if err := json.Unmarshal(b, &basePL); err != nil {
		log.WithError(err).Error("api/roaming: unmarshal request error")
		return
	}

	// set ctx id
	ctxID, err := uuid.NewV4()
	if err != nil {
		a.writeResponse(r.Context(), w, a.getBasePayloadResult(basePL, backend.Other, fmt.Sprintf("get uuid error: %s", err)))
		return
	}
	ctx := context.WithValue(r.Context(), logging.ContextIDKey, ctxID)

	// validate SenderID
	if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
		if r.TLS.PeerCertificates[0].Subject.CommonName != basePL.SenderID {
			log.WithFields(log.Fields{
				"ctx_id":      ctx.Value(logging.ContextIDKey),
				"sender_id":   basePL.SenderID,
				"common_name": r.TLS.PeerCertificates[0].Subject.CommonName,
			}).Error("roaming: SenderID does not match client-certificate CommonName")
			a.writeResponse(ctx, w, a.getBasePayloadResult(basePL, backend.UnknownSender, "SenderID does not match client-certificate CommonName"))
			return
		}
	}

	// validate ReceiverID
	if basePL.ReceiverID != a.netID.String() {
		log.WithFields(log.Fields{
			"ctx_id":      ctx.Value(logging.ContextIDKey),
			"receiver_id": basePL.ReceiverID,
			"net_id":      a.netID.String(),
		}).Error("roaming: ReceiverID does not match NetID of network-server")
		a.writeResponse(ctx, w, a.getBasePayloadResult(basePL, backend.UnknownReceiver, "ReceiverID does not match NetID of network-server"))
		return
	}

	log.WithFields(log.Fields{
		"ctx_id":       ctx.Value(logging.ContextIDKey),
		"message_type": basePL.MessageType,
		"sender_id":    basePL.SenderID,
		"receiver_id":  basePL.ReceiverID,
	}).Info("roaming: api request received")

	// handle request
	switch basePL.MessageType {
	case backend.PRStartReq:
		a.handlePRStartReq(ctx, w, basePL, b)
	case backend.PRStopReq:
		a.handlePRStopReq(ctx, w, b)
	case backend.ProfileReq:
		a.handleProfileReq(ctx, w, b)
	case backend.XmitDataReq:
		a.handleXmitDataReq(ctx, w, basePL, b)
	default:
		a.writeResponse(ctx, w, a.getBasePayloadResult(basePL, backend.MalformedRequest, fmt.Sprintf("MessageType %s is not expected", basePL.MessageType)))
	}
}

func (a *API) handlePRStartReq(ctx context.Context, w http.ResponseWriter, basePL backend.BasePayload, b []byte) {
	var pl backend.PRStartReqPayload
	if err := json.Unmarshal(b, &pl); err != nil {
		a.writeResponse(ctx, w, a.getBasePayloadResult(basePL, backend.MalformedRequest, fmt.Sprintf("unmarshal json error: %s", err)))
		return
	}

	// decode requester netid
	var netID lorawan.NetID
	if err := netID.UnmarshalText([]byte(basePL.SenderID)); err != nil {
		a.writeResponse(ctx, w, a.getBasePayloadResult(basePL, backend.MalformedRequest, fmt.Sprintf("unmarshal netid error: %s", err)))
		return
	}

	// frequency in hz
	var freq int
	if pl.ULMetaData.ULFreq != nil {
		freq = int(*pl.ULMetaData.ULFreq * 1000000)
	}

	// data-rate
	var dr int
	if pl.ULMetaData.DataRate != nil {
		dr = *pl.ULMetaData.DataRate
	}

	// channel index
	ch, err := band.Band().GetUplinkChannelIndexForFrequencyDR(freq, dr)
	if err != nil {
		a.writeResponse(ctx, w, a.getBasePayloadResult(basePL, backend.MalformedRequest, fmt.Sprintf("get channel error: %s", err)))
		return
	}

	// phypayload
	var phy lorawan.PHYPayload
	if err := phy.UnmarshalBinary(pl.PHYPayload[:]); err != nil {
		a.writeResponse(ctx, w, a.getBasePayloadResult(basePL, backend.MalformedRequest, fmt.Sprintf("unmarshal phypayload error: %s", err)))
		return
	}

	// get device-session
	ds, err := storage.GetDeviceSessionForPHYPayload(ctx, phy, dr, ch)
	if err != nil {
		if err == storage.ErrInvalidMIC || err == storage.ErrFrameCounterRetransmission {
			a.writeResponse(ctx, w, a.getBasePayloadResult(basePL, backend.MICFailed, err.Error()))
		} else if err == storage.ErrDoesNotExist {
			a.writeResponse(ctx, w, a.getBasePayloadResult(basePL, backend.UnknownDevAddr, err.Error()))
		} else {
			a.writeResponse(ctx, w, a.getBasePayloadResult(basePL, backend.Other, err.Error()))
		}

		return
	}

	// lifetime
	lifetime := int(roaming.GetPassiveRoamingLifetime(netID) / time.Second)

	// sess keys
	kekLabel := roaming.GetPassiveRaomingKEKLabel(netID)
	var kekKey []byte
	if kekLabel != "" {
		kekKey, err = roaming.GetKEKKey(kekLabel)
		if err != nil {
			a.writeResponse(ctx, w, a.getBasePayloadResult(basePL, backend.Other, err.Error()))
			return
		}
	}
	var fNwkSIntKey *backend.KeyEnvelope
	var nwkSKey *backend.KeyEnvelope

	if ds.GetMACVersion() == lorawan.LoRaWAN1_0 {
		nwkSKey, err = backend.NewKeyEnvelope(kekLabel, kekKey, ds.NwkSEncKey)
		if err != nil {
			a.writeResponse(ctx, w, a.getBasePayloadResult(basePL, backend.Other, err.Error()))
			return
		}
	} else {
		fNwkSIntKey, err = backend.NewKeyEnvelope(kekLabel, kekKey, ds.FNwkSIntKey)
		if err != nil {
			a.writeResponse(ctx, w, a.getBasePayloadResult(basePL, backend.Other, err.Error()))
			return
		}
	}

	ans := backend.PRStartAnsPayload{
		BasePayloadResult: a.getBasePayloadResult(basePL, backend.Success, ""),
		DevEUI:            &ds.DevEUI,
		Lifetime:          &lifetime,
		FNwkSIntKey:       fNwkSIntKey,
		NwkSKey:           nwkSKey,
		FCntUp:            &ds.FCntUp,
	}

	a.writeResponse(ctx, w, ans)
}

func (a *API) handlePRStopReq(ctx context.Context, w http.ResponseWriter, pl []byte) {

}

func (a *API) handleProfileReq(ctx context.Context, w http.ResponseWriter, pl []byte) {

}

func (a *API) handleXmitDataReq(ctx context.Context, w http.ResponseWriter, basePL backend.BasePayload, b []byte) {
	var pl backend.XmitDataReqPayload
	if err := json.Unmarshal(b, &pl); err != nil {
		a.writeResponse(ctx, w, a.getBasePayloadResult(basePL, backend.MalformedRequest, fmt.Sprintf("unmarshal json error: %s", err)))
		return
	}

	if len(pl.PHYPayload[:]) != 0 && pl.ULMetaData != nil {
		// Passive Roaming
		// In this case we receive the PHYPayload together with ULMetaData

		// Decode PHYPayload
		var phy lorawan.PHYPayload
		if err := phy.UnmarshalBinary(pl.PHYPayload[:]); err != nil {
			a.writeResponse(ctx, w, a.getBasePayloadResult(basePL, backend.Other, fmt.Sprintf("unmarshal phypayload error: %s", err)))
			return
		}

		// Convert ULMetaData to UplinkRXInfo and UplinkTXInfo
		txInfo, err := roaming.ULMetaDataToTXInfo(*pl.ULMetaData)
		if err != nil {
			a.writeResponse(ctx, w, a.getBasePayloadResult(basePL, backend.Other, fmt.Sprintf("ul meta-data to txinfo error: %s", err)))
			return
		}
		rxInfo, err := roaming.ULMetaDataToRXInfo(*pl.ULMetaData)
		if err != nil {
			a.writeResponse(ctx, w, a.getBasePayloadResult(basePL, backend.Other, fmt.Sprintf("ul meta-data to rxinfo error: %s", err)))
			return
		}

		// Construct RXPacket
		rxPacket := models.RXPacket{
			PHYPayload: phy,
			TXInfo:     txInfo,
			RXInfoSet:  rxInfo,
		}
		if pl.ULMetaData.DataRate != nil {
			rxPacket.DR = *pl.ULMetaData.DataRate
		}

		// Start the uplink data flow
		if err := data.Handle(ctx, rxPacket); err != nil {
			a.writeResponse(ctx, w, a.getBasePayloadResult(basePL, backend.Other, fmt.Sprintf("handle uplink error: %s", err)))
			return
		}
	} else {
		a.writeResponse(ctx, w, a.getBasePayloadResult(basePL, backend.MalformedRequest, "unexpected payload"))
		return
	}

	a.writeResponse(ctx, w, a.getBasePayloadResult(basePL, backend.Success, ""))
}

func (a *API) getBasePayloadResult(basePLReq backend.BasePayload, resCode backend.ResultCode, resDesc string) backend.BasePayloadResult {
	var mType backend.MessageType

	switch basePLReq.MessageType {
	case backend.PRStartReq:
		mType = backend.PRStartAns
	case backend.PRStopReq:
		mType = backend.PRStopAns
	case backend.ProfileReq:
		mType = backend.ProfileAns
	case backend.XmitDataReq:
		mType = backend.XmitDataAns
	}

	return backend.BasePayloadResult{
		BasePayload: backend.BasePayload{
			ProtocolVersion: backend.ProtocolVersion1_0,
			SenderID:        basePLReq.ReceiverID,
			ReceiverID:      basePLReq.SenderID,
			TransactionID:   basePLReq.TransactionID,
			MessageType:     mType,
		},
		Result: backend.Result{
			ResultCode:  resCode,
			Description: resDesc,
		},
	}
}

func (a *API) writeResponse(ctx context.Context, w http.ResponseWriter, v backend.Answer) error {
	log.WithFields(log.Fields{
		"ctx_id":       ctx.Value(logging.ContextIDKey),
		"sender_id":    v.GetBasePayload().SenderID,
		"receiver_id":  v.GetBasePayload().ReceiverID,
		"message_type": v.GetBasePayload().MessageType,
		"result_code":  v.GetBasePayload().Result.ResultCode,
	}).Info("api/roaming: returning api response")

	b, err := json.Marshal(v)
	if err != nil {
		return errors.Wrap(err, "marshal json error")
	}

	_, err = w.Write(b)
	if err != nil {
		return errors.Wrap(err, "write response error")
	}

	return nil
}
