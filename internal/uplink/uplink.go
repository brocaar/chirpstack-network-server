package uplink

import (
	"context"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/gofrs/uuid"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/chirpstack-api/go/v3/common"
	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/brocaar/chirpstack-api/go/v3/nc"
	"github.com/brocaar/chirpstack-api/go/v3/ns"
	"github.com/brocaar/chirpstack-network-server/v3/internal/backend/controller"
	gwbackend "github.com/brocaar/chirpstack-network-server/v3/internal/backend/gateway"
	"github.com/brocaar/chirpstack-network-server/v3/internal/config"
	"github.com/brocaar/chirpstack-network-server/v3/internal/downlink/ack"
	"github.com/brocaar/chirpstack-network-server/v3/internal/framelog"
	"github.com/brocaar/chirpstack-network-server/v3/internal/gateway"
	"github.com/brocaar/chirpstack-network-server/v3/internal/helpers"
	"github.com/brocaar/chirpstack-network-server/v3/internal/logging"
	"github.com/brocaar/chirpstack-network-server/v3/internal/models"
	"github.com/brocaar/chirpstack-network-server/v3/internal/storage"
	"github.com/brocaar/chirpstack-network-server/v3/internal/uplink/data"
	"github.com/brocaar/chirpstack-network-server/v3/internal/uplink/join"
	"github.com/brocaar/chirpstack-network-server/v3/internal/uplink/proprietary"
	"github.com/brocaar/chirpstack-network-server/v3/internal/uplink/rejoin"
	"github.com/brocaar/lorawan"
)

var (
	deduplicationDelay              time.Duration
	ignoreFrequencyForDeduplication bool
)

// Setup configures the package.
func Setup(conf config.Config) error {
	if err := data.Setup(conf); err != nil {
		return errors.Wrap(err, "configure uplink/data error")
	}

	if err := join.Setup(conf); err != nil {
		return errors.Wrap(err, "configure uplink/join error")
	}

	if err := rejoin.Setup(conf); err != nil {
		return errors.Wrap(err, "configure uplink/rejoin error")
	}

	deduplicationDelay = conf.NetworkServer.DeduplicationDelay

	ignoreFrequencyForDeduplication = conf.NetworkServer.IgnoreFrequencyForDeduplication
	if ignoreFrequencyForDeduplication {
		log.Warn("Ignoring the frequency of the packets when deduplicating: This introduces a security issue that enables a re-play attack - https://github.com/brocaar/chirpstack-network-server/issues/557#issuecomment-968719234")
	}

	return nil
}

// Server represents a server listening for uplink packets.
type Server struct {
	wg sync.WaitGroup
}

// NewServer creates a new server.
func NewServer() *Server {
	return &Server{}
}

// Start starts the server.
func (s *Server) Start() error {
	go func() {
		s.wg.Add(1)
		defer s.wg.Done()
		HandleUplinkFrames(&s.wg)
	}()

	go func() {
		s.wg.Add(1)
		defer s.wg.Done()
		HandleDownlinkTXAcks(&s.wg)
	}()
	return nil
}

// Stop closes the gateway backend and waits for the server to complete the
// pending packets.
func (s *Server) Stop() error {
	if err := gwbackend.Backend().Close(); err != nil {
		return fmt.Errorf("close gateway backend error: %s", err)
	}
	log.Info("uplink: waiting for pending actions to complete")
	s.wg.Wait()
	return nil
}

// HandleUplinkFrames consumes received packets by the gateway and handles them
// in a separate go-routine. Errors are logged.
func HandleUplinkFrames(wg *sync.WaitGroup) {
	for uplinkFrame := range gwbackend.Backend().RXPacketChan() {
		go func(uplinkFrame gw.UplinkFrame) {
			wg.Add(1)
			defer wg.Done()

			// The ctxID will be available as context value "ctx_id" so that
			// this can be used when writing logs. This makes it easier to
			// group multiple log-lines to the same context.
			ctxID, err := uuid.NewV4()
			if err != nil {
				log.WithError(err).Error("uplink: get new uuid error")
			}

			ctx := context.Background()
			ctx = context.WithValue(ctx, logging.ContextIDKey, ctxID)

			if err := HandleUplinkFrame(ctx, uplinkFrame); err != nil {
				log.WithFields(log.Fields{
					"ctx_id": ctxID,
				}).WithError(err).Error("uplink: processing uplink frame error")
			}
		}(uplinkFrame)
	}
}

// HandleUplinkFrame handles a single uplink frame.
func HandleUplinkFrame(ctx context.Context, uplinkFrame gw.UplinkFrame) error {
	return collectUplinkFrames(ctx, uplinkFrame)
}

// HandleDownlinkTXAcks consumes received downlink tx acknowledgements from
// the gateway.
func HandleDownlinkTXAcks(wg *sync.WaitGroup) {
	for downlinkTXAck := range gwbackend.Backend().DownlinkTXAckChan() {
		go func(downlinkTXAck gw.DownlinkTXAck) {
			wg.Add(1)
			defer wg.Done()

			// The ctxID will be available as context value "ctx_id" so that
			// this can be used when writing logs. This makes it easier to
			// group multiple log-lines to the same context.
			var ctxID uuid.UUID
			if downlinkTXAck.DownlinkId != nil {
				copy(ctxID[:], downlinkTXAck.DownlinkId)
			}

			ctx := context.Background()
			ctx = context.WithValue(ctx, logging.ContextIDKey, ctxID)

			if err := ack.HandleDownlinkTXAck(ctx, &downlinkTXAck); err != nil {
				log.WithFields(log.Fields{
					"gateway_id": hex.EncodeToString(downlinkTXAck.GatewayId),
					"token":      downlinkTXAck.Token,
					"ctx_id":     ctxID,
				}).WithError(err).Error("uplink: handle downlink tx ack error")
			}

		}(downlinkTXAck)
	}
}

func collectUplinkFrames(ctx context.Context, uplinkFrame gw.UplinkFrame) error {
	return collectAndCallOnce(uplinkFrame, ignoreFrequencyForDeduplication, func(rxPacket models.RXPacket) error {
		err := handleCollectedUplink(ctx, uplinkFrame, rxPacket)
		if err != nil {
			cause := errors.Cause(err)
			if cause == storage.ErrDoesNotExist || cause == storage.ErrFrameCounterReset || cause == storage.ErrInvalidMIC || cause == storage.ErrFrameCounterRetransmission {
				if _, err := controller.Client().HandleRejectedUplinkFrameSet(ctx, &nc.HandleRejectedUplinkFrameSetRequest{
					FrameSet: &gw.UplinkFrameSet{
						PhyPayload: uplinkFrame.PhyPayload,
						TxInfo:     rxPacket.TXInfo,
						RxInfo:     rxPacket.RXInfoSet,
					},
				}); err != nil {
					log.WithError(err).Error("uplink: call controller HandleRejectedUplinkFrameSet RPC error")
				}
			}
		}

		return err
	})
}
func runHandlerWithMetric(err error, mt lorawan.MType) error {
	mts := mt.String()
	if err != nil {
		uplinkFrameCounter(mts + "Err").Inc()
		return err
	}

	uplinkFrameCounter(mts).Inc()

	return err
}

func handleCollectedUplink(ctx context.Context, uplinkFrame gw.UplinkFrame, rxPacket models.RXPacket) error {
	// Update the gateway meta-data.
	// This sets the location information from the database, decrypts the
	// fine-timestamp when it is available and retrieves the service-profile
	// information of the gateways.
	// Note: this is done after de-duplication as a single uplink might be
	// received multiple times in case of multiple NS instances and depending
	// the MQTT broker (e.g. if it supports consumer groups).
	if err := gateway.UpdateMetaDataInRXPacket(ctx, storage.DB(), &rxPacket); err != nil {
		return errors.Wrap(err, "update RXPacket meta-data error")
	}

	// Return if the RXInfoSet is empty.
	if len(rxPacket.RXInfoSet) == 0 {
		return nil
	}

	var uplinkIDs []uuid.UUID
	for _, p := range rxPacket.RXInfoSet {
		uplinkIDs = append(uplinkIDs, helpers.GetUplinkID(p))
	}

	log.WithFields(log.Fields{
		"uplink_ids": uplinkIDs,
		"mtype":      rxPacket.PHYPayload.MHDR.MType,
		"ctx_id":     ctx.Value(logging.ContextIDKey),
	}).Info("uplink: frame(s) collected")

	// Extract MType
	var protoMType common.MType
	switch rxPacket.PHYPayload.MHDR.MType {
	case lorawan.JoinRequest:
		protoMType = common.MType_JoinRequest
	case lorawan.RejoinRequest:
		protoMType = common.MType_RejoinRequest
	case lorawan.UnconfirmedDataUp:
		protoMType = common.MType_UnconfirmedDataUp
	case lorawan.ConfirmedDataUp:
		protoMType = common.MType_ConfirmedDataUp
	case lorawan.Proprietary:
		protoMType = common.MType_Proprietary
	}

	// Extract DevAddr or DevEUI (if available)
	var devAddr []byte
	var devEUI []byte
	switch v := rxPacket.PHYPayload.MACPayload.(type) {
	case *lorawan.MACPayload:
		devAddr = v.FHDR.DevAddr[:]
	case *lorawan.JoinRequestPayload:
		devEUI = v.DevEUI[:]
	case *lorawan.RejoinRequestType02Payload:
		devEUI = v.DevEUI[:]
	case *lorawan.RejoinRequestType1Payload:
		devEUI = v.DevEUI[:]
	}

	// log the frame for each receiving gateway.
	if err := framelog.LogUplinkFrameForGateways(ctx, ns.UplinkFrameLog{
		PhyPayload: uplinkFrame.PhyPayload,
		TxInfo:     rxPacket.TXInfo,
		RxInfo:     rxPacket.RXInfoSet,
		MType:      protoMType,
		DevAddr:    devAddr,
		DevEui:     devEUI,
	}); err != nil {
		log.WithFields(log.Fields{
			"ctx_id": ctx.Value(logging.ContextIDKey),
		}).WithError(err).Error("uplink: log uplink frames for gateways error")
	}

	// handle the frame based on message-type
	switch rxPacket.PHYPayload.MHDR.MType {
	case lorawan.JoinRequest:
		return runHandlerWithMetric(join.Handle(ctx, rxPacket), lorawan.JoinRequest)
	case lorawan.RejoinRequest:
		return runHandlerWithMetric(rejoin.Handle(ctx, rxPacket), lorawan.RejoinRequest)
	case lorawan.UnconfirmedDataUp, lorawan.ConfirmedDataUp:
		return runHandlerWithMetric(data.Handle(ctx, rxPacket), lorawan.UnconfirmedDataUp)
	case lorawan.Proprietary:
		return runHandlerWithMetric(proprietary.Handle(ctx, rxPacket), lorawan.Proprietary)
	default:
		return nil
	}
}
