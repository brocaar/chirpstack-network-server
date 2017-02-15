package stats

import (
	"log"
	"sync"

	"github.com/brocaar/loraserver/internal/common"
)

// Server represents a server listening for gw stats packets.
type Server struct {
	ctx common.Context
	wg  sync.WaitGroup
}

// NewServer creates a new Server.
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
		HandleStatsPackets(&s.wg, s.ctx)
	}()
	return nil
}

// Stop waits for the stats server to complete the pending packets.
// At this stage the gateway backend is already closed.
func (s *Server) Stop() error {
	s.wg.Wait()
	return nil
}

// HandleStatsPackets consumes received stats packets by the gateway and
// update the statistics.
func HandleStatsPackets(wg *sync.WaitGroup, ctx common.Context) {
	for statsPacket := range ctx.Gateway.StatsPacketChan() {
		log.Println(statsPacket)
	}
}
