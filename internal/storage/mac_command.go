package storage

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"

	"github.com/go-redis/redis/v7"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/chirpstack-network-server/internal/logging"
	"github.com/brocaar/lorawan"
)

const (
	macCommandQueueTempl   = "lora:ns:device:%s:mac:queue"
	macCommandPendingTempl = "lora:ns:device:%s:mac:pending:%d"
)

// MACCommandBlock defines a block of MAC commands that must be handled
// together.
type MACCommandBlock struct {
	CID         lorawan.CID
	External    bool // command was enqueued by an external service
	MACCommands MACCommands
}

// Size returns the size (in bytes) of the mac-commands within this block.
func (m *MACCommandBlock) Size() (int, error) {
	var count int
	for _, mc := range m.MACCommands {
		b, err := mc.MarshalBinary()
		if err != nil {
			return 0, errors.Wrap(err, "marshal binary error")
		}
		count += len(b)
	}
	return count, nil
}

// MACCommands holds a slice of MACCommand items.
type MACCommands []lorawan.MACCommand

// MarshalBinary implements the encoding.BinaryMarshaler interface.
func (m MACCommands) MarshalBinary() ([]byte, error) {
	var out []byte
	for _, mac := range m {
		b, err := mac.MarshalBinary()
		if err != nil {
			return nil, err
		}
		out = append(out, b...)
	}
	return out, nil
}

// UnmarshalBinary implements the encoding.BinaryUnmarshaler interface.
func (m *MACCommands) UnmarshalBinary(data []byte) error {
	var pLen int
	for i := 0; i < len(data); i++ {
		if _, s, err := lorawan.GetMACPayloadAndSize(false, lorawan.CID(data[i])); err != nil {
			pLen = 0
		} else {
			pLen = s
		}

		// check if the remaining bytes are >= CID byte + payload size
		if len(data[i:]) < pLen+1 {
			return errors.New("not enough remaining bytes")
		}

		var mc lorawan.MACCommand
		if err := mc.UnmarshalBinary(false, data[i:i+1+pLen]); err != nil {
			return err
		}
		*m = append(*m, mc)
		i += pLen
	}
	return nil
}

// FlushMACCommandQueue flushes the mac-command queue for the given DevEUI.
func FlushMACCommandQueue(ctx context.Context, devEUI lorawan.EUI64) error {
	key := fmt.Sprintf(macCommandQueueTempl, devEUI)

	err := RedisClient().Del(key).Err()
	if err != nil {
		return errors.Wrap(err, "flush mac-command queue error")
	}

	return nil
}

// CreateMACCommandQueueItem creates a new mac-command queue item.
func CreateMACCommandQueueItem(ctx context.Context, devEUI lorawan.EUI64, block MACCommandBlock) error {
	key := fmt.Sprintf(macCommandQueueTempl, devEUI)

	var buf bytes.Buffer
	err := gob.NewEncoder(&buf).Encode(block)
	if err != nil {
		return errors.Wrap(err, "gob encode error")
	}

	pipe := RedisClient().TxPipeline()
	pipe.RPush(key, buf.Bytes())
	pipe.PExpire(key, deviceSessionTTL)

	_, err = pipe.Exec()
	if err != nil {
		return errors.Wrap(err, "create mac-command queue item error")
	}

	log.WithFields(log.Fields{
		"dev_eui": devEUI,
		"cid":     block.CID,
		"ctx_id":  ctx.Value(logging.ContextIDKey),
	}).Info("mac-command queue item created")

	return nil
}

// GetMACCommandQueueItems returns the mac-command queue items for the
// given DevEUI.
func GetMACCommandQueueItems(ctx context.Context, devEUI lorawan.EUI64) ([]MACCommandBlock, error) {
	var out []MACCommandBlock
	key := fmt.Sprintf(macCommandQueueTempl, devEUI)

	values, err := RedisClient().LRange(key, 0, -1).Result()
	if err != nil {
		return nil, errors.Wrap(err, "get mac-command queue items error")
	}

	for _, value := range values {
		var block MACCommandBlock
		err = gob.NewDecoder(bytes.NewReader([]byte(value))).Decode(&block)
		if err != nil {
			return nil, errors.Wrap(err, "gob decode error")
		}

		out = append(out, block)
	}

	return out, nil
}

// DeleteMACCommandQueueItem deletes the given mac-command from the queue.
func DeleteMACCommandQueueItem(ctx context.Context, devEUI lorawan.EUI64, block MACCommandBlock) error {
	key := fmt.Sprintf(macCommandQueueTempl, devEUI)

	var buf bytes.Buffer
	err := gob.NewEncoder(&buf).Encode(block)
	if err != nil {
		return errors.Wrap(err, "gob encode error")
	}

	val, err := RedisClient().LRem(key, 0, buf.Bytes()).Result()
	if err != nil {
		return errors.Wrap(err, "delete mac-command queue item error")
	}

	if val == 0 {
		return ErrDoesNotExist
	}

	log.WithFields(log.Fields{
		"dev_eui": devEUI,
		"cid":     block.CID,
		"ctx_id":  ctx.Value(logging.ContextIDKey),
	}).Info("mac-command deleted from queue")

	return nil
}

// SetPendingMACCommand sets a mac-command to the pending buffer.
// In case an other mac-command with the same CID has been set to pending,
// it will be overwritten.
func SetPendingMACCommand(ctx context.Context, devEUI lorawan.EUI64, block MACCommandBlock) error {
	key := fmt.Sprintf(macCommandPendingTempl, devEUI, block.CID)

	var buf bytes.Buffer
	err := gob.NewEncoder(&buf).Encode(block)
	if err != nil {
		return errors.Wrap(err, "gob encode error")
	}

	err = RedisClient().Set(key, buf.Bytes(), deviceSessionTTL).Err()
	if err != nil {
		return errors.Wrap(err, "set mac-command pending error")
	}

	log.WithFields(log.Fields{
		"dev_eui":  devEUI,
		"cid":      block.CID,
		"commands": len(block.MACCommands),
		"ctx_id":   ctx.Value(logging.ContextIDKey),
	}).Info("pending mac-command block set")

	return nil
}

// GetPendingMACCommand returns the pending mac-command for the given CID.
// In case no items are pending, nil is returned.
func GetPendingMACCommand(ctx context.Context, devEUI lorawan.EUI64, cid lorawan.CID) (*MACCommandBlock, error) {
	var block MACCommandBlock
	key := fmt.Sprintf(macCommandPendingTempl, devEUI, cid)

	val, err := RedisClient().Get(key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, errors.Wrap(err, "get pending mac-command error")
	}

	err = gob.NewDecoder(bytes.NewReader(val)).Decode(&block)
	if err != nil {
		return nil, errors.Wrap(err, "gob decode error")
	}

	return &block, nil
}

// DeletePendingMACCommand removes the pending mac-command for the given CID.
func DeletePendingMACCommand(ctx context.Context, devEUI lorawan.EUI64, cid lorawan.CID) error {
	key := fmt.Sprintf(macCommandPendingTempl, devEUI, cid)

	val, err := RedisClient().Del(key).Result()
	if err != nil {
		return errors.Wrap(err, "delete pending mac-command error")
	}
	if val == 0 {
		return ErrDoesNotExist
	}

	log.WithFields(log.Fields{
		"dev_eui": devEUI,
		"cid":     cid,
		"ctx_id":  ctx.Value(logging.ContextIDKey),
	}).Info("pending mac-command deleted")

	return nil
}
