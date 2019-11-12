package gateway

import (
	"context"
	"crypto/aes"
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	"github.com/gofrs/uuid"
	"github.com/golang/protobuf/ptypes"
	"github.com/gomodule/redigo/redis"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/chirpstack-api/go/common"
	"github.com/brocaar/chirpstack-api/go/gw"
	"github.com/brocaar/chirpstack-network-server/internal/backend/gateway"
	"github.com/brocaar/chirpstack-network-server/internal/gateway/stats"
	"github.com/brocaar/chirpstack-network-server/internal/helpers"
	"github.com/brocaar/chirpstack-network-server/internal/logging"
	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/brocaar/lorawan"
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

// UpdateMetaDataInRxInfoSet updates the gateway meta-data in the
// given rx-info set. It will:
//   - add the gateway location
//   - set the FPGA id if available
//   - decrypt the fine-timestamp (if available and AES key is set)
func UpdateMetaDataInRxInfoSet(ctx context.Context, db sqlx.Queryer, p *redis.Pool, rxInfo []*gw.UplinkRXInfo) error {
	for i := range rxInfo {
		id := helpers.GetGatewayID(rxInfo[i])
		g, err := storage.GetAndCacheGateway(ctx, db, p, id)
		if err != nil {
			log.WithFields(log.Fields{
				"ctx_id":     ctx.Value(logging.ContextIDKey),
				"gateway_id": id,
			}).WithError(err).Error("get gateway error")
			continue
		}

		// set gateway location
		rxInfo[i].Location = &common.Location{
			Latitude:  g.Location.Latitude,
			Longitude: g.Location.Longitude,
			Altitude:  g.Altitude,
		}

		var board storage.GatewayBoard
		if int(rxInfo[i].Board) < len(g.Boards) {
			board = g.Boards[int(rxInfo[i].Board)]
		}

		// set FPGA ID
		// this is useful when the AES decryption key is not set as it
		// indicates which key to use for decryption
		if rxInfo[i].FineTimestampType == gw.FineTimestampType_ENCRYPTED && board.FPGAID != nil {
			tsInfo := rxInfo[i].GetEncryptedFineTimestamp()
			if tsInfo == nil {
				log.WithFields(log.Fields{
					"ctx_id":     ctx.Value(logging.ContextIDKey),
					"gateway_id": id,
				}).Error("encrypted_fine_timestamp must not be nil")
				continue
			}

			if len(tsInfo.FpgaId) == 0 {
				tsInfo.FpgaId = board.FPGAID[:]
			}
		}

		// decrypt fine-timestamp when the AES key is known
		if rxInfo[i].FineTimestampType == gw.FineTimestampType_ENCRYPTED && board.FineTimestampKey != nil {
			tsInfo := rxInfo[i].GetEncryptedFineTimestamp()
			if tsInfo == nil {
				log.WithFields(log.Fields{
					"ctx_id":     ctx.Value(logging.ContextIDKey),
					"gateway_id": id,
				}).Error("encrypted_fine_timestamp must not be nil")
				continue
			}

			if rxInfo[i].Time == nil {
				log.WithFields(log.Fields{
					"ctx_id":     ctx.Value(logging.ContextIDKey),
					"gateway_id": id,
				}).Error("time must not be nil")
				continue
			}

			rxTime, err := ptypes.Timestamp(rxInfo[i].Time)
			if err != nil {
				log.WithFields(log.Fields{
					"ctx_id":     ctx.Value(logging.ContextIDKey),
					"gateway_id": id,
				}).WithError(err).Error("get timestamp error")
			}

			plainTS, err := decryptFineTimestamp(*board.FineTimestampKey, rxTime, *tsInfo)
			if err != nil {
				log.WithFields(log.Fields{
					"ctx_id":     ctx.Value(logging.ContextIDKey),
					"gateway_id": id,
				}).WithError(err).Error("decrypt fine-timestamp error")
				continue
			}

			rxInfo[i].FineTimestampType = gw.FineTimestampType_PLAIN
			rxInfo[i].FineTimestamp = &gw.UplinkRXInfo_PlainFineTimestamp{
				PlainFineTimestamp: &plainTS,
			}
		}
	}

	return nil
}

func decryptFineTimestamp(key lorawan.AES128Key, rxTime time.Time, ts gw.EncryptedFineTimestamp) (gw.PlainFineTimestamp, error) {
	var plainTS gw.PlainFineTimestamp

	block, err := aes.NewCipher(key[:])
	if err != nil {
		return plainTS, errors.Wrap(err, "new cipher error")
	}

	if len(ts.EncryptedNs) != block.BlockSize() {
		return plainTS, fmt.Errorf("invalid block-size (%d) or ciphertext length (%d)", block.BlockSize(), len(ts.EncryptedNs))
	}

	ct := make([]byte, block.BlockSize())
	block.Decrypt(ct, ts.EncryptedNs)

	nanoSec := binary.BigEndian.Uint64(ct[len(ct)-8:])
	nanoSec = nanoSec / 32

	if time.Duration(nanoSec) >= time.Second {
		return plainTS, errors.New("expected fine-timestamp nanosecond remainder must be < 1 second, did you set the correct decryption key?")
	}

	rxTime = rxTime.Add(time.Duration(nanoSec) * time.Nanosecond)

	plainTS.Time, err = ptypes.TimestampProto(rxTime)
	if err != nil {
		return plainTS, errors.Wrap(err, "timestamp proto error")
	}

	return plainTS, nil
}
