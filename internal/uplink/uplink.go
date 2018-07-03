package uplink

import (
	"fmt"
	"sync"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/framelog"
	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/uplink/data"
	"github.com/brocaar/loraserver/internal/uplink/join"
	"github.com/brocaar/loraserver/internal/uplink/proprietary"
	"github.com/brocaar/loraserver/internal/uplink/rejoin"
	"github.com/brocaar/lorawan"
)

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
	return nil
}

// Stop closes the gateway backend and waits for the server to complete the
// pending packets.
func (s *Server) Stop() error {
	if err := config.C.NetworkServer.Gateway.Backend.Backend.Close(); err != nil {
		return fmt.Errorf("close gateway backend error: %s", err)
	}
	log.Info("waiting for pending actions to complete")
	s.wg.Wait()
	return nil
}

// HandleRXPackets consumes received packets by the gateway and handles them
// in a separate go-routine. Errors are logged.
func HandleRXPackets(wg *sync.WaitGroup) {
	for rxPacket := range config.C.NetworkServer.Gateway.Backend.Backend.RXPacketChan() {
		go func(rxPacket gw.RXPacket) {
			wg.Add(1)
			defer wg.Done()
			if err := HandleRXPacket(rxPacket); err != nil {
				data, _ := rxPacket.PHYPayload.MarshalText()
				log.WithField("data_base64", string(data)).Errorf("processing rx packet error: %s", err)
			}
		}(rxPacket)
	}
}

// HandleRXPacket handles a single rxpacket.
func HandleRXPacket(rxPacket gw.RXPacket) error {
	return collectPackets(rxPacket)
}

func collectPackets(rxPacket gw.RXPacket) error {
	return collectAndCallOnce(config.C.Redis.Pool, rxPacket, func(rxPacket models.RXPacket) error {
		uplinkFrameSet, err := framelog.CreateUplinkFrameSet(rxPacket)
		if err != nil {
			return errors.Wrap(err, "create uplink frame-set error")
		}

		if err := framelog.LogUplinkFrameForGateways(uplinkFrameSet); err != nil {
			log.WithError(err).Error("log uplink frames for gateways error")
		}

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
