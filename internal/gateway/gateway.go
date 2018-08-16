package gateway

import (
	"sync"

	log "github.com/sirupsen/logrus"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/storage"
)

// StatsHandler represents a stat handler for incoming gateway stats.
type StatsHandler struct {
	wg sync.WaitGroup
}

// NewStatsHandler creates a new StatsHandler.
func NewStatsHandler() *StatsHandler {
	return &StatsHandler{}
}

// Start starts the stats handler.
func (s *StatsHandler) Start() error {
	go func() {
		s.wg.Add(1)
		defer s.wg.Done()
		handleStatsPackets(&s.wg)
	}()
	return nil
}

// Stop waits for the stats handler to complete the pending packets.
// At this stage the gateway backend must already been closed.
func (s *StatsHandler) Stop() error {
	s.wg.Wait()
	return nil
}

// handleStatsPackets consumes received stats packets by the gateway.
func handleStatsPackets(wg *sync.WaitGroup) {
	for statsPacket := range config.C.NetworkServer.Gateway.Backend.Backend.StatsPacketChan() {
		go func(stats gw.GatewayStats) {
			wg.Add(1)
			defer wg.Done()
			if err := storage.HandleGatewayStatsPacket(config.C.PostgreSQL.DB, stats); err != nil {
				log.Errorf("handle stats packet error: %s", err)
			}
		}(statsPacket)
	}
}
