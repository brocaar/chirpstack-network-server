package gateway

import (
	"context"
	"sync"

	"github.com/gofrs/uuid"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/brocaar/chirpstack-network-server/internal/backend/gateway"
	"github.com/brocaar/chirpstack-network-server/internal/gateway/stats"
	"github.com/brocaar/chirpstack-network-server/internal/logging"
)

// StatsHandler represents a stat handler for incoming gateway stats.
type StatsHandler struct {
	wg sync.WaitGroup
}

// Start starts the stats handler.
func (s *StatsHandler) Start() error {
	go func() {
		s.wg.Add(1)
		defer s.wg.Done()

		if gateway.Backend() == nil {
			return
		}

		for gwStats := range gateway.Backend().StatsPacketChan() {
			go func(gwStats gw.GatewayStats) {
				s.wg.Add(1)
				defer s.wg.Done()

				var statsID uuid.UUID
				if gwStats.StatsId != nil {
					copy(statsID[:], gwStats.StatsId)
				}

				ctx := context.Background()
				ctx = context.WithValue(ctx, logging.ContextIDKey, statsID)

				if err := stats.Handle(ctx, gwStats); err != nil {
					log.WithError(err).WithFields(log.Fields{
						"ctx_id": ctx.Value(logging.ContextIDKey),
					}).Error("gateway: handle gateway stats error")
				}

			}(gwStats)
		}
	}()
	return nil
}

// Stop waits for the stats handler to complete the pending packets.
// At this stage the gateway backend must already been closed.
func (s *StatsHandler) Stop() error {
	s.wg.Wait()
	return nil
}
