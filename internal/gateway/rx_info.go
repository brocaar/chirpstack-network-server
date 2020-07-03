package gateway

import (
	"context"
	"crypto/aes"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/chirpstack-api/go/v3/common"
	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/brocaar/chirpstack-network-server/internal/helpers"
	"github.com/brocaar/chirpstack-network-server/internal/logging"
	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/brocaar/lorawan"
)

// UpdateMetaDataInRxInfoSet updates the gateway meta-data in the
// given rx-info set. It will:
//   - add the gateway location
//   - set the FPGA id if available
//   - decrypt the fine-timestamp (if available and AES key is set)
func UpdateMetaDataInRxInfoSet(ctx context.Context, db sqlx.Queryer, rxInfo []*gw.UplinkRXInfo) error {
	for i := range rxInfo {
		id := helpers.GetGatewayID(rxInfo[i])
		g, err := storage.GetAndCacheGateway(ctx, db, id)
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
