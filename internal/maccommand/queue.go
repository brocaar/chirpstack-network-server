package maccommand

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/garyburd/redigo/redis"
	"github.com/pkg/errors"

	"github.com/Frankz/loraserver/internal/common"
	"github.com/Frankz/lorawan"
)

const (
	queueTempl   = "lora:ns:mac:queue:%s"
	pendingTempl = "lora:ns:mac:pending:%s:%d"
)

// FlushQueue flushes the mac-payload queue for the given devEUI.
func FlushQueue(p *redis.Pool, devEUI lorawan.EUI64) error {
	c := p.Get()
	defer c.Close()

	key := fmt.Sprintf(queueTempl, devEUI)
	_, err := redis.Int(c.Do("DEL", key))
	if err != nil {
		return errors.Wrap(err, "flush queue error")
	}
	return nil
}

// FilterItems filters the given slice of MACCommandBlock elements based
// on the given criteria (FRMPayload and max-bytes).
func FilterItems(blocks []Block, frmPayload bool, maxBytes int) ([]Block, error) {
	var out []Block
	var count int
	for _, b := range blocks {
		if b.FRMPayload == frmPayload {
			c, err := b.Size()
			if err != nil {
				return nil, errors.Wrap(err, "get size error")
			}
			count += c
			if count > maxBytes {
				return out, nil
			}
			out = append(out, b)
		}
	}
	return out, nil
}

// AddQueueItem adds the given mac-command block to the queue.
// In case a mac-command block for the same CID exists, the old mac-command
// block will be removed.
func AddQueueItem(p *redis.Pool, devEUI lorawan.EUI64, block Block) error {
	if err := DeleteQueueItemByCID(p, devEUI, block.CID); err != nil {
		return errors.Wrap(err, "delete queue item error")
	}

	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(block); err != nil {
		return errors.Wrap(err, "gob encode error")
	}

	c := p.Get()
	defer c.Close()

	exp := int64(common.NodeSessionTTL) / int64(time.Millisecond)
	key := fmt.Sprintf(queueTempl, devEUI)

	c.Send("MULTI")
	c.Send("RPUSH", key, buf.Bytes())
	c.Send("PEXPIRE", key, exp)
	_, err := c.Do("EXEC")
	if err != nil {
		return errors.Wrap(err, "add mac-command block to queue error")
	}

	log.WithFields(log.Fields{
		"dev_eui":    devEUI,
		"frmpayload": block.FRMPayload,
		"cid":        block.CID,
	}).Info("mac-command block added to queue")
	return nil
}

// ReadQueueItems returns all mac command blocks.
func ReadQueueItems(p *redis.Pool, devEUI lorawan.EUI64) ([]Block, error) {
	var out []Block

	c := p.Get()
	defer c.Close()

	key := fmt.Sprintf(queueTempl, devEUI)
	values, err := redis.Values(c.Do("LRANGE", key, 0, -1))
	if err != nil {
		return nil, errors.Wrap(err, "read mac-command queue error")
	}

	for _, value := range values {
		b, ok := value.([]byte)
		if !ok {
			return nil, fmt.Errorf("expected []byte type, got %T", value)
		}

		var block Block
		err = gob.NewDecoder(bytes.NewReader(b)).Decode(&block)
		if err != nil {
			return nil, errors.Wrap(err, "decode mac-command block error")
		}
		out = append(out, block)
	}
	return out, nil
}

// GetQueueItemByCID returns the queue item matching the given CID.
func GetQueueItemByCID(p *redis.Pool, devEUI lorawan.EUI64, cid lorawan.CID) (*Block, error) {
	queue, err := ReadQueueItems(p, devEUI)
	if err != nil {
		return nil, errors.Wrap(err, "read queue error")
	}

	for _, block := range queue {
		if block.CID == cid {
			return &block, nil
		}
	}

	return nil, nil
}

// DeleteQueueItemByCID deletes the mac-commands matching the given CID from
// the queue. No error is returned when the given CID does not match any
// item in the queue.
func DeleteQueueItemByCID(p *redis.Pool, devEUI lorawan.EUI64, cid lorawan.CID) error {
	queue, err := ReadQueueItems(p, devEUI)
	if err != nil {
		return errors.Wrap(err, "read queue error")
	}

	for _, block := range queue {
		if block.CID == cid {
			if err = DeleteQueueItem(p, devEUI, block); err != nil {
				return errors.Wrap(err, "delete queue item error")
			}
		}
	}

	return nil
}

// DeleteQueueItem deletes the given mac-command block from the queue.
func DeleteQueueItem(p *redis.Pool, devEUI lorawan.EUI64, block Block) error {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(block); err != nil {
		return errors.Wrap(err, "gob encode mac-command block error")
	}

	c := p.Get()
	defer c.Close()

	key := fmt.Sprintf(queueTempl, devEUI)
	val, err := redis.Int(c.Do("LREM", key, 0, buf.Bytes()))
	if err != nil {
		return errors.Wrap(err, "delete mac-command block from queue error")
	}

	if val == 0 {
		return ErrDoesNotExist
	}

	log.WithFields(log.Fields{
		"dev_eui": devEUI,
		"cid":     block.CID,
	}).Info("mac-command block removed from queue")
	return nil
}

// SetPending sets a MACCommandBlock to the pending buffer.
// In case an other MACCommandBlock with the same CID has been set to pending,
// it will be overwritten.
func SetPending(p *redis.Pool, devEUI lorawan.EUI64, block Block) error {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(block); err != nil {
		return errors.Wrap(err, "gob encode mac-command block error")
	}

	c := p.Get()
	defer c.Close()

	key := fmt.Sprintf(pendingTempl, devEUI, block.CID)
	exp := int64(common.NodeSessionTTL) / int64(time.Millisecond)

	_, err := c.Do("PSETEX", key, exp, buf.Bytes())
	if err != nil {
		return errors.Wrap(err, "write mac-command blocks to pending queue error")
	}

	log.WithFields(log.Fields{
		"dev_eui":     devEUI,
		"cid":         block.CID,
		"frm_payload": block.FRMPayload,
		"commands":    len(block.MACCommands),
	}).Info("pending mac-command block set")

	return nil
}

// ReadPending returns the pending MACCommandBlock for the given CID.
// In case no items are pending, nil is returned.
func ReadPending(p *redis.Pool, devEUI lorawan.EUI64, cid lorawan.CID) (*Block, error) {
	var block Block

	c := p.Get()
	defer c.Close()

	key := fmt.Sprintf(pendingTempl, devEUI, cid)
	val, err := redis.Bytes(c.Do("GET", key))
	if err != nil {
		if err == redis.ErrNil {
			return nil, nil
		}
		return nil, errors.Wrap(err, "get mac-command block error")
	}

	if err := gob.NewDecoder(bytes.NewReader(val)).Decode(&block); err != nil {
		return nil, errors.Wrap(err, "decode mac-command block error")
	}

	return &block, nil
}

// DeletePending removes the pending MACCommandBlock for the given CID.
func DeletePending(p *redis.Pool, devEUI lorawan.EUI64, cid lorawan.CID) error {
	c := p.Get()
	defer c.Close()

	key := fmt.Sprintf(pendingTempl, devEUI, cid)
	val, err := redis.Int(c.Do("DEL", key))
	if err != nil {
		return errors.Wrap(err, "delete mac-command block from pending error")
	}
	if val == 0 {
		return ErrDoesNotExist
	}
	log.WithFields(log.Fields{
		"dev_eui": devEUI,
		"cid":     cid,
	}).Info("mac-command block removed from pending")
	return nil
}
