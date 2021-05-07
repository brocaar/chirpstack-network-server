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

	"github.com/gofrs/uuid"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/chirpstack-network-server/v3/internal/band"
	"github.com/brocaar/chirpstack-network-server/v3/internal/config"
	downdata "github.com/brocaar/chirpstack-network-server/v3/internal/downlink/data"
	"github.com/brocaar/chirpstack-network-server/v3/internal/logging"
	"github.com/brocaar/chirpstack-network-server/v3/internal/models"
	"github.com/brocaar/chirpstack-network-server/v3/internal/roaming"
	"github.com/brocaar/chirpstack-network-server/v3/internal/storage"
	updata "github.com/brocaar/chirpstack-network-server/v3/internal/uplink/data"
	"github.com/brocaar/chirpstack-network-server/v3/internal/uplink/join"
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
		Handler: &API{
			netID: c.NetworkServer.NetID,
		},
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

// NewAPI creates a new API.
func NewAPI(netID lorawan.NetID) *API {
	return &API{
		netID: netID,
	}
}

// ServeHTTP handles a HTTP request.
func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// read request body
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.WithError(err).Error("api/roaming: read request body error")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// decode the BasePayload part of the request
	var basePL backend.BasePayload
	if err := json.Unmarshal(b, &basePL); err != nil {
		log.WithError(err).Error("api/roaming: unmarshal request error")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// set ctx id
	ctxID, err := uuid.NewV4()
	if err != nil {
		log.WithError(err).Error("api/roaming: get uuid error")
		w.WriteHeader(http.StatusInternalServerError)
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
			w.WriteHeader(http.StatusUnauthorized)
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
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Get NetID from SenderID
	var netID lorawan.NetID
	if err := netID.UnmarshalText([]byte(basePL.SenderID)); err != nil {
		log.WithFields(log.Fields{
			"ctx_id":    ctx.Value(logging.ContextIDKey),
			"sender_id": basePL.SenderID,
		}).WithError(err).Error("api/roaming: unmarshal SenderID as NetID error")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Get ClientID for NetID
	client, err := roaming.GetClientForNetID(netID)
	if err != nil {
		log.WithFields(log.Fields{
			"ctx_id":    ctx.Value(logging.ContextIDKey),
			"sender_id": basePL.SenderID,
		}).WithError(err).Error("api/roaming: get client for NetID error")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	log.WithFields(log.Fields{
		"ctx_id":         ctx.Value(logging.ContextIDKey),
		"message_type":   basePL.MessageType,
		"sender_id":      basePL.SenderID,
		"receiver_id":    basePL.ReceiverID,
		"async_client":   client.IsAsync(),
		"transaction_id": basePL.TransactionID,
	}).Info("roaming: api request received")

	// handle request
	if client.IsAsync() {
		newCtx := context.Background()
		newCtx = context.WithValue(newCtx, logging.ContextIDKey, ctx.Value(logging.ContextIDKey))

		go a.handleAsync(newCtx, client, basePL, b)
	} else {
		a.handleSync(ctx, client, w, basePL, b)
	}
}

func (a *API) handleAsync(ctx context.Context, client backend.Client, basePL backend.BasePayload, b []byte) {
	ans, err := a.handleRequest(ctx, client, basePL, b)
	if err != nil {

		ans = a.getBasePayloadResult(basePL, a.errToResultCode(err), err.Error())
	}

	if ans == nil {
		return
	}

	if err := client.SendAnswer(ctx, ans); err != nil {
		log.WithError(err).WithFields(log.Fields{
			"ctx_id":         ctx.Value(logging.ContextIDKey),
			"sender_id":      client.GetSenderID(),
			"receiver_id":    client.GetReceiverID(),
			"transaction_id": ans.GetBasePayload().TransactionID,
		}).Error("api/roaming: sending async response error")
	} else {
		log.WithFields(log.Fields{
			"ctx_id":         ctx.Value(logging.ContextIDKey),
			"sender_id":      ans.GetBasePayload().SenderID,
			"receiver_id":    ans.GetBasePayload().ReceiverID,
			"message_type":   ans.GetBasePayload().MessageType,
			"result_code":    ans.GetBasePayload().Result.ResultCode,
			"transaction_id": ans.GetBasePayload().TransactionID,
		}).Info("api/roaming: returned async response")
	}
}

func (a *API) handleSync(ctx context.Context, client backend.Client, w http.ResponseWriter, basePL backend.BasePayload, b []byte) {
	ans, err := a.handleRequest(ctx, client, basePL, b)
	if err != nil {
		ans = a.getBasePayloadResult(basePL, a.errToResultCode(err), err.Error())
	}

	if ans == nil {
		return
	}

	bb, err := json.Marshal(ans)
	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"ctx_id":         ctx.Value(logging.ContextIDKey),
			"sender_id":      ans.GetBasePayload().SenderID,
			"receiver_id":    ans.GetBasePayload().ReceiverID,
			"message_type":   ans.GetBasePayload().MessageType,
			"result_code":    ans.GetBasePayload().Result.ResultCode,
			"transaction_id": ans.GetBasePayload().TransactionID,
		}).Error("api/roaming: marshal json error")
		return
	}

	_, err = w.Write(bb)
	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"ctx_id":         ctx.Value(logging.ContextIDKey),
			"sender_id":      ans.GetBasePayload().SenderID,
			"receiver_id":    ans.GetBasePayload().ReceiverID,
			"message_type":   ans.GetBasePayload().MessageType,
			"result_code":    ans.GetBasePayload().Result.ResultCode,
			"transaction_id": ans.GetBasePayload().TransactionID,
		}).Error("api/roaming: write respone error")
	} else {
		log.WithFields(log.Fields{
			"ctx_id":         ctx.Value(logging.ContextIDKey),
			"sender_id":      ans.GetBasePayload().SenderID,
			"receiver_id":    ans.GetBasePayload().ReceiverID,
			"message_type":   ans.GetBasePayload().MessageType,
			"result_code":    ans.GetBasePayload().Result.ResultCode,
			"transaction_id": ans.GetBasePayload().TransactionID,
		}).Info("api/roaming: returned response")
	}
}

func (a *API) handleRequest(ctx context.Context, client backend.Client, basePL backend.BasePayload, b []byte) (backend.Answer, error) {
	var ans backend.Answer
	var err error

	switch basePL.MessageType {
	case backend.PRStartReq:
		ans, err = a.handlePRStartReq(ctx, basePL, b)
	case backend.PRStartAns:
		err = a.handlePRStartAns(ctx, client, basePL, b)
	case backend.PRStopReq:
		ans, err = a.handlePRStopReq(ctx, basePL, b)
	case backend.PRStopAns:
		err = a.handlePRStopAns(ctx, client, basePL, b)
	case backend.ProfileReq:
		ans, err = a.handleProfileReq(ctx, basePL, b)
	case backend.ProfileAns:
		err = a.handleProfileAns(ctx, client, basePL, b)
	case backend.XmitDataReq:
		ans, err = a.handleXmitDataReq(ctx, basePL, b)
	case backend.XmitDataAns:
		err = a.handleXmitDataAns(ctx, client, basePL, b)
	default:
		ans = a.getBasePayloadResult(basePL, backend.MalformedRequest, fmt.Sprintf("MessageType %s is not expected", basePL.MessageType))
	}

	return ans, err
}

func (a *API) handlePRStartAns(ctx context.Context, client backend.Client, basePL backend.BasePayload, b []byte) error {
	var pl backend.PRStartAnsPayload
	if err := json.Unmarshal(b, &pl); err != nil {
		return errors.Wrap(err, "unmarshal json error")
	}

	if err := client.HandleAnswer(ctx, pl); err != nil {
		return errors.Wrap(err, "handle answer error")
	}

	return nil
}

func (a *API) handlePRStartReq(ctx context.Context, basePL backend.BasePayload, b []byte) (backend.Answer, error) {
	// payload
	var pl backend.PRStartReqPayload
	if err := json.Unmarshal(b, &pl); err != nil {
		return nil, errors.Wrap(err, "unmarshal json error")
	}

	// phypayload
	var phy lorawan.PHYPayload
	if err := phy.UnmarshalBinary(pl.PHYPayload[:]); err != nil {
		return nil, errors.Wrap(err, "unmarshal phypayload error")
	}

	if phy.MHDR.MType == lorawan.JoinRequest {
		return a.handlePRStartReqJoin(ctx, basePL, pl, phy)
	} else {
		return a.handlePRStartReqData(ctx, basePL, pl, phy)
	}
}

func (a *API) handlePRStartReqData(ctx context.Context, basePL backend.BasePayload, pl backend.PRStartReqPayload, phy lorawan.PHYPayload) (backend.Answer, error) {
	// decode requester netid
	var netID lorawan.NetID
	if err := netID.UnmarshalText([]byte(basePL.SenderID)); err != nil {
		return nil, errors.Wrap(err, "unmarshal netid error")
	}

	// frequency in hz
	var freq uint32
	if pl.ULMetaData.ULFreq != nil {
		freq = uint32(*pl.ULMetaData.ULFreq * 1000000)
	}

	// data-rate
	var dr int
	if pl.ULMetaData.DataRate != nil {
		dr = *pl.ULMetaData.DataRate
	}

	// channel index
	ch, err := band.Band().GetUplinkChannelIndexForFrequencyDR(freq, dr)
	if err != nil {
		return nil, errors.Wrap(err, "get uplink channel index for frequency and dr error")
	}

	// get device-session
	ds, err := storage.GetDeviceSessionForPHYPayload(ctx, phy, dr, ch)
	if err != nil {
		return nil, errors.Wrap(err, "get device-session for phypayload error")
	}

	// lifetime
	lifetime := int(roaming.GetPassiveRoamingLifetime(netID) / time.Second)
	var fNwkSIntKey *backend.KeyEnvelope
	var nwkSKey *backend.KeyEnvelope

	if lifetime != 0 {
		// sess keys
		kekLabel := roaming.GetPassiveRoamingKEKLabel(netID)
		var kekKey []byte
		if kekLabel != "" {
			kekKey, err = roaming.GetKEKKey(kekLabel)
			if err != nil {
				return nil, errors.Wrap(err, "get kek key error")
			}
		}

		if ds.GetMACVersion() == lorawan.LoRaWAN1_0 {
			nwkSKey, err = backend.NewKeyEnvelope(kekLabel, kekKey, ds.NwkSEncKey)
			if err != nil {
				return nil, errors.Wrap(err, "new key envelope error")
			}
		} else {
			fNwkSIntKey, err = backend.NewKeyEnvelope(kekLabel, kekKey, ds.FNwkSIntKey)
			if err != nil {
				return nil, errors.Wrap(err, "new key envelope error")
			}
		}
	} else {
		// In case of stateless, the payload is directly handled
		if err := updata.HandleRoamingHNS(ctx, pl.PHYPayload[:], pl.BasePayload, pl.ULMetaData); err != nil {
			return nil, errors.Wrap(err, "handle passive-roaming uplink error")
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

	return ans, nil
}

func (a *API) handlePRStartReqJoin(ctx context.Context, basePL backend.BasePayload, pl backend.PRStartReqPayload, phy lorawan.PHYPayload) (backend.Answer, error) {
	rxInfo, err := roaming.ULMetaDataToRXInfo(pl.ULMetaData)
	if err != nil {
		return nil, errors.Wrap(err, "ULMetaData to RXInfo error")
	}

	txInfo, err := roaming.ULMetaDataToTXInfo(pl.ULMetaData)
	if err != nil {
		return nil, errors.Wrap(err, "ULMetaData to TXInfo error")
	}

	rxPacket := models.RXPacket{
		PHYPayload: phy,
		TXInfo:     txInfo,
		RXInfoSet:  rxInfo,
	}
	if pl.ULMetaData.DataRate != nil {
		rxPacket.DR = *pl.ULMetaData.DataRate
	}

	ans, err := join.HandleStartPRHNS(ctx, pl, rxPacket)
	if err != nil {
		return nil, errors.Wrap(err, "handle otaa error")
	}

	bpl := a.getBasePayloadResult(basePL, backend.Success, "")
	ans.BasePayload = bpl.BasePayload
	ans.Result = bpl.Result
	return ans, nil
}

func (a *API) handlePRStopAns(ctx context.Context, client backend.Client, basePL backend.BasePayload, b []byte) error {
	var pl backend.PRStopAnsPayload
	if err := json.Unmarshal(b, &pl); err != nil {
		return errors.Wrap(err, "unmarshal json error")
	}

	if err := client.HandleAnswer(ctx, pl); err != nil {
		return errors.Wrap(err, "handle answer error")
	}

	return nil
}

func (a *API) handlePRStopReq(ctx context.Context, basePL backend.BasePayload, pl []byte) (backend.Answer, error) {
	return nil, nil
}

func (a *API) handleProfileAns(ctx context.Context, client backend.Client, basePL backend.BasePayload, b []byte) error {
	var pl backend.ProfileAnsPayload
	if err := json.Unmarshal(b, &pl); err != nil {
		return errors.Wrap(err, "unmarshal json error")
	}

	if err := client.HandleAnswer(ctx, pl); err != nil {
		return errors.Wrap(err, "handle answer error")
	}

	return nil
}

func (a *API) handleProfileReq(ctx context.Context, basePL backend.BasePayload, pl []byte) (backend.Answer, error) {
	return nil, nil
}

func (a *API) handleXmitDataAns(ctx context.Context, client backend.Client, basePL backend.BasePayload, b []byte) error {
	var pl backend.XmitDataAnsPayload
	if err := json.Unmarshal(b, &pl); err != nil {
		return errors.Wrap(err, "unmarshal json error")
	}

	if err := client.HandleAnswer(ctx, pl); err != nil {
		return errors.Wrap(err, "handle answer error")
	}

	return nil
}

func (a *API) handleXmitDataReq(ctx context.Context, basePL backend.BasePayload, b []byte) (backend.Answer, error) {
	var pl backend.XmitDataReqPayload
	if err := json.Unmarshal(b, &pl); err != nil {
		return nil, errors.Wrap(err, "unmarshal json error")
	}

	if len(pl.PHYPayload[:]) != 0 && pl.ULMetaData != nil {
		// Passive Roaming uplink
		if err := updata.HandleRoamingHNS(ctx, pl.PHYPayload[:], pl.BasePayload, *pl.ULMetaData); err != nil {
			return nil, errors.Wrap(err, "handle passive-roaming uplink error")
		}
	} else if len(pl.PHYPayload[:]) != 0 && pl.DLMetaData != nil {
		// Passive Roaming downlink
		if err := downdata.HandleRoamingFNS(ctx, pl); err != nil {
			return nil, errors.Wrap(err, "handle passive-roaming downlink error")
		}
	} else {
		return nil, errors.New("unexpected payload")
	}

	return a.getBasePayloadResult(basePL, backend.Success, ""), nil
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
			ProtocolVersion: basePLReq.ProtocolVersion,
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

func (a *API) errToResultCode(err error) backend.ResultCode {
	return backend.Other
}
