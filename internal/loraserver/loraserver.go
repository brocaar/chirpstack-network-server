package loraserver

import (
	"fmt"
	"sync"

	log "github.com/Sirupsen/logrus"
	"github.com/brocaar/loraserver/models"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/band"
)

// Band is the ISM band configuration to use
var Band band.Band

// Server represents a LoRaWAN network-server.
type Server struct {
	ctx Context
	wg  sync.WaitGroup
}

// NewServer creates a new server.
func NewServer(ctx Context) *Server {
	return &Server{
		ctx: ctx,
	}
}

// Start starts the server.
func (s *Server) Start() error {
	go func() {
		s.wg.Add(1)
		handleRXPackets(s.ctx)
		s.wg.Done()
	}()
	go func() {
		s.wg.Add(1)
		handleTXPayloads(s.ctx)
		s.wg.Done()
	}()
	return nil
}

// Stop closes the gateway and application backends and waits for the
// server to complete the pending packets and actions.
func (s *Server) Stop() error {
	if err := s.ctx.Gateway.Close(); err != nil {
		return fmt.Errorf("close gateway backend error: %s", err)
	}
	if err := s.ctx.Application.Close(); err != nil {
		return fmt.Errorf("close application backend error: %s", err)
	}
	if err := s.ctx.Controller.Close(); err != nil {
		return fmt.Errorf("close network-controller backend error: %s", err)
	}

	log.Info("waiting for pending packets to complete")
	s.wg.Wait()
	return nil
}

func handleTXPayloads(ctx Context) {
	var wg sync.WaitGroup
	for txPayload := range ctx.Application.TXPayloadChan() {
		go func(txPayload models.TXPayload) {
			wg.Add(1)
			if err := addTXPayloadToQueue(ctx.RedisPool, txPayload); err != nil {
				log.WithField("dev_eui", txPayload.DevEUI).Errorf("add tx payload to queue error: %s", err)
			}
			wg.Done()
		}(txPayload)
	}
	wg.Wait()
}

func handleRXPackets(ctx Context) {
	var wg sync.WaitGroup
	for rxPacket := range ctx.Gateway.RXPacketChan() {
		go func(rxPacket models.RXPacket) {
			wg.Add(1)
			if err := handleRXPacket(ctx, rxPacket); err != nil {
				data, _ := rxPacket.PHYPayload.MarshalText()
				log.WithField("data_base64", string(data)).Errorf("processing rx packet error: %s", err)
			}
			wg.Done()
		}(rxPacket)
	}
	wg.Wait()
}

func handleRXPacket(ctx Context, rxPacket models.RXPacket) error {
	switch rxPacket.PHYPayload.MHDR.MType {
	case lorawan.JoinRequest:
		return validateAndCollectJoinRequestPacket(ctx, rxPacket)
	case lorawan.UnconfirmedDataUp, lorawan.ConfirmedDataUp:
		return validateAndCollectDataUpRXPacket(ctx, rxPacket)
	default:
		return fmt.Errorf("unknown MType: %v", rxPacket.PHYPayload.MHDR.MType)
	}
}
