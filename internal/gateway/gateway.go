package gateway

import (
	"crypto/aes"
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/gomodule/redigo/redis"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/loraserver/api/common"
	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/helpers"
	"github.com/brocaar/loraserver/internal/storage"
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
				log.WithError(err).Error("handle stats packet error")
			}

			var gatewayID lorawan.EUI64
			copy(gatewayID[:], stats.GatewayId)
			if err := storage.FlushGatewayCache(config.C.Redis.Pool, gatewayID); err != nil {
				log.WithError(err).Error("flush gateway cache error")
			}
		}(statsPacket)
	}
}

// UpdateMetaDataInRxInfoSet updates the gateway meta-data in the
// given rx-info set. It will:
//   - add the gateway location
//   - set the FPGA id if available
//   - decrypt the fine-timestamp (if available and AES key is set)
func UpdateMetaDataInRxInfoSet(db sqlx.Queryer, p *redis.Pool, rxInfo []*gw.UplinkRXInfo) error {
	for i := range rxInfo {
		id := helpers.GetGatewayID(rxInfo[i])
		g, err := storage.GetAndCacheGateway(db, p, id)
		if err != nil {
			log.WithFields(log.Fields{
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
					"gateway_id": id,
				}).Error("encrypted_fine_timestamp must not be nil")
				continue
			}

			if rxInfo[i].Time == nil {
				log.WithFields(log.Fields{
					"gateway_id": id,
				}).Error("time must not be nil")
				continue
			}

			rxTime, err := ptypes.Timestamp(rxInfo[i].Time)
			if err != nil {
				log.WithFields(log.Fields{
					"gateway_id": id,
				}).WithError(err).Error("get timestamp error")
			}

			plainTS, err := decryptFineTimestamp(*board.FineTimestampKey, rxTime, *tsInfo)
			if err != nil {
				log.WithFields(log.Fields{
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
