package uplink

import (
	"fmt"
	"sync"

	log "github.com/sirupsen/logrus"

	"github.com/Frankz/loraserver/api/gw"
	"github.com/Frankz/loraserver/internal/common"
	"github.com/Frankz/loraserver/internal/models"
	"github.com/Frankz/loraserver/internal/uplink/data"
	"github.com/Frankz/loraserver/internal/uplink/join"
	"github.com/Frankz/loraserver/internal/uplink/proprietary"
	"github.com/Frankz/lorawan"
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
	if err := common.Gateway.Close(); err != nil {
		return fmt.Errorf("close gateway backend error: %s", err)
	}
	log.Info("waiting for pending actions to complete")
	s.wg.Wait()
	return nil
}

// HandleRXPackets consumes received packets by the gateway and handles them
// in a separate go-routine. Errors are logged.
func HandleRXPackets(wg *sync.WaitGroup) {
	for rxPacket := range common.Gateway.RXPacketChan() {
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
	return collectAndCallOnce(common.RedisPool, rxPacket, func(rxPacket models.RXPacket) error {
		switch rxPacket.PHYPayload.MHDR.MType {
		case lorawan.JoinRequest:
			return join.Handle(rxPacket)
		case lorawan.UnconfirmedDataUp, lorawan.ConfirmedDataUp:
			return data.Handle(rxPacket)
		case lorawan.Proprietary:
			return proprietary.Handle(rxPacket)
		default:
			return nil
		}
	})
}
