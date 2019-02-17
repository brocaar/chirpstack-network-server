package uplink

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/loraserver/api/gw"
	gwbackend "github.com/brocaar/loraserver/internal/backend/gateway"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/downlink/ack"
	"github.com/brocaar/loraserver/internal/framelog"
	"github.com/brocaar/loraserver/internal/gateway"
	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/loraserver/internal/uplink/data"
	"github.com/brocaar/loraserver/internal/uplink/join"
	"github.com/brocaar/loraserver/internal/uplink/proprietary"
	"github.com/brocaar/loraserver/internal/uplink/rejoin"
	"github.com/brocaar/lorawan"
)

var (
	deduplicationDelay time.Duration
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
		HandleRXPackets(&s.wg)
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
	log.Info("waiting for pending actions to complete")
	s.wg.Wait()
	return nil
}

// HandleRXPackets consumes received packets by the gateway and handles them
// in a separate go-routine. Errors are logged.
func HandleRXPackets(wg *sync.WaitGroup) {
	for uplinkFrame := range gwbackend.Backend().RXPacketChan() {
		go func(uplinkFrame gw.UplinkFrame) {
			wg.Add(1)
			defer wg.Done()
			if err := HandleRXPacket(uplinkFrame); err != nil {
				data := base64.StdEncoding.EncodeToString(uplinkFrame.PhyPayload)
				log.WithField("data_base64", data).WithError(err).Error("processing uplink frame error")
			}
		}(uplinkFrame)
	}
}

// HandleRXPacket handles a single rxpacket.
func HandleRXPacket(uplinkFrame gw.UplinkFrame) error {
	return collectPackets(uplinkFrame)
}

// HandleDownlinkTXAcks consumes received downlink tx acknowledgements from
// the gateway.
func HandleDownlinkTXAcks(wg *sync.WaitGroup) {
	for downlinkTXAck := range gwbackend.Backend().DownlinkTXAckChan() {
		go func(downlinkTXAck gw.DownlinkTXAck) {
			wg.Add(1)
			defer wg.Done()
			if err := ack.HandleDownlinkTXAck(downlinkTXAck); err != nil {
				log.WithFields(log.Fields{
					"gateway_id": hex.EncodeToString(downlinkTXAck.GatewayId),
					"token":      downlinkTXAck.Token,
				}).WithError(err).Error("handle downlink tx ack error")
			}

		}(downlinkTXAck)
	}
}

func collectPackets(uplinkFrame gw.UplinkFrame) error {
	return collectAndCallOnce(storage.RedisPool(), uplinkFrame, func(rxPacket models.RXPacket) error {
		// update the gateway meta-data
		if err := gateway.UpdateMetaDataInRxInfoSet(storage.DB(), storage.RedisPool(), rxPacket.RXInfoSet); err != nil {
			log.WithError(err).Error("update gateway meta-data in rx-info set error")
		}

		// log the frame for each receiving gatewa
		if err := framelog.LogUplinkFrameForGateways(storage.RedisPool(), gw.UplinkFrameSet{
			PhyPayload: uplinkFrame.PhyPayload,
			TxInfo:     rxPacket.TXInfo,
			RxInfo:     rxPacket.RXInfoSet,
		}); err != nil {
			log.WithError(err).Error("log uplink frames for gateways error")
		}

		// handle the frame based on message-type
		switch rxPacket.PHYPayload.MHDR.MType {
		case lorawan.JoinRequest:
			return join.Handle(rxPacket)
		case lorawan.RejoinRequest:
			return rejoin.Handle(rxPacket)
		case lorawan.UnconfirmedDataUp, lorawan.ConfirmedDataUp:
			return data.Handle(rxPacket)
		case lorawan.Proprietary:
			return proprietary.Handle(rxPacket)
		default:
			return nil
		}
	})
}
