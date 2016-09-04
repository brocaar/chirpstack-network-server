package uplink

import (
	"fmt"
	"sync"

	log "github.com/Sirupsen/logrus"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/lorawan"
)

// Server represents a server listening for uplink packets.
type Server struct {
	ctx common.Context
	wg  sync.WaitGroup
}

// NewServer creates a new server.
func NewServer(ctx common.Context) *Server {
	return &Server{
		ctx: ctx,
	}
}

// Start starts the server.
func (s *Server) Start() error {
	go func() {
		s.wg.Add(1)
		defer s.wg.Done()
		HandleRXPackets(&s.wg, s.ctx)
	}()
	return nil
}

// Stop closes the gateway backend and waits for the server to complete the
// pending packets.
func (s *Server) Stop() error {
	if err := s.ctx.Gateway.Close(); err != nil {
		return fmt.Errorf("close gateway backend error: %s", err)
	}
	log.Info("waiting for pending actions to complete")
	s.wg.Wait()
	return nil
}

// HandleRXPackets consumes received packets by the gateway and handles them
// in a separate go-routine. Errors are logged.
func HandleRXPackets(wg *sync.WaitGroup, ctx common.Context) {
	for rxPacket := range ctx.Gateway.RXPacketChan() {
		go func(rxPacket gw.RXPacket) {
			wg.Add(1)
			defer wg.Done()
			if err := HandleRXPacket(ctx, rxPacket); err != nil {
				data, _ := rxPacket.PHYPayload.MarshalText()
				log.WithField("data_base64", string(data)).Errorf("processing rx packet error: %s", err)
			}
		}(rxPacket)
	}
}

// HandleRXPacket handles a single rxpacket.
func HandleRXPacket(ctx common.Context, rxPacket gw.RXPacket) error {
	switch rxPacket.PHYPayload.MHDR.MType {
	case lorawan.JoinRequest:
		return collectJoinRequestPacket(ctx, rxPacket)
	case lorawan.UnconfirmedDataUp, lorawan.ConfirmedDataUp:
		return validateAndCollectDataUpRXPacket(ctx, rxPacket)
	default:
		return fmt.Errorf("unknown MType: %v", rxPacket.PHYPayload.MHDR.MType)
	}
}
