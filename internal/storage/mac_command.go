package storage

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/loraserver/internal/config"
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
func FlushMACCommandQueue(p *redis.Pool, devEUI lorawan.EUI64) error {
	c := p.Get()
	defer c.Close()

	key := fmt.Sprintf(macCommandQueueTempl, devEUI)
	_, err := redis.Int(c.Do("DEL", key))
	if err != nil {
		return errors.Wrap(err, "flush mac-command queue error")
	}
	return nil
}

// CreateMACCommandQueueItem creates a new mac-command queue item.
func CreateMACCommandQueueItem(p *redis.Pool, devEUI lorawan.EUI64, block MACCommandBlock) error {
	var buf bytes.Buffer
	err := gob.NewEncoder(&buf).Encode(block)
	if err != nil {
		return errors.Wrap(err, "gob encode error")
	}

	c := p.Get()
	defer c.Close()

	exp := int64(config.C.NetworkServer.DeviceSessionTTL) / int64(time.Millisecond)
	key := fmt.Sprintf(macCommandQueueTempl, devEUI)

	c.Send("MULTI")
	c.Send("RPUSH", key, buf.Bytes())
	c.Send("PEXPIRE", key, exp)
	_, err = c.Do("EXEC")
	if err != nil {
		return errors.Wrap(err, "create mac-command queue item error")
	}

	log.WithFields(log.Fields{
		"dev_eui": devEUI,
		"cid":     block.CID,
	}).Info("mac-command queue item created")

	return nil
}

// GetMACCommandQueueItems returns the mac-command queue items for the
// given DevEUI.
func GetMACCommandQueueItems(p *redis.Pool, devEUI lorawan.EUI64) ([]MACCommandBlock, error) {
	var out []MACCommandBlock

	c := p.Get()
	defer c.Close()

	key := fmt.Sprintf(macCommandQueueTempl, devEUI)
	values, err := redis.Values(c.Do("LRANGE", key, 0, -1))
	if err != nil {
		return nil, errors.Wrap(err, "get mac-command queue items error")
	}

	for _, value := range values {
		b, ok := value.([]byte)
		if !ok {
			return nil, fmt.Errorf("expecte []byte type, got %T", value)
		}

		var block MACCommandBlock
		err = gob.NewDecoder(bytes.NewReader(b)).Decode(&block)
		if err != nil {
			return nil, errors.Wrap(err, "gob decode error")
		}

		out = append(out, block)
	}

	return out, nil
}

// DeleteMACCommandQueueItem deletes the given mac-command from the queue.
func DeleteMACCommandQueueItem(p *redis.Pool, devEUI lorawan.EUI64, block MACCommandBlock) error {
	var buf bytes.Buffer
	err := gob.NewEncoder(&buf).Encode(block)
	if err != nil {
		return errors.Wrap(err, "gob encode error")
	}

	c := p.Get()
	defer c.Close()

	key := fmt.Sprintf(macCommandQueueTempl, devEUI)
	val, err := redis.Int(c.Do("LREM", key, 0, buf.Bytes()))
	if err != nil {
		return errors.Wrap(err, "delete mac-command queue item error")
	}

	if val == 0 {
		return ErrDoesNotExist
	}

	log.WithFields(log.Fields{
		"dev_eui": devEUI,
		"cid":     block.CID,
	}).Info("mac-command deleted from queue")

	return nil
}

// SetPendingMACCommand sets a mac-command to the pending buffer.
// In case an other mac-command with the same CID has been set to pending,
// it will be overwritten.
func SetPendingMACCommand(p *redis.Pool, devEUI lorawan.EUI64, block MACCommandBlock) error {
	var buf bytes.Buffer
	err := gob.NewEncoder(&buf).Encode(block)
	if err != nil {
		return errors.Wrap(err, "gob encode error")
	}

	c := p.Get()
	defer c.Close()

	key := fmt.Sprintf(macCommandPendingTempl, devEUI, block.CID)
	exp := int64(config.C.NetworkServer.DeviceSessionTTL) / int64(time.Millisecond)

	_, err = c.Do("PSETEX", key, exp, buf.Bytes())
	if err != nil {
		return errors.Wrap(err, "set mac-command pending error")
	}

	log.WithFields(log.Fields{
		"dev_eui":  devEUI,
		"cid":      block.CID,
		"commands": len(block.MACCommands),
	}).Info("pending mac-command block set")

	return nil
}

// GetPendingMACCommand returns the pending mac-command for the given CID.
// In case no items are pending, nil is returned.
func GetPendingMACCommand(p *redis.Pool, devEUI lorawan.EUI64, cid lorawan.CID) (*MACCommandBlock, error) {
	var block MACCommandBlock

	c := p.Get()
	defer c.Close()

	key := fmt.Sprintf(macCommandPendingTempl, devEUI, cid)
	val, err := redis.Bytes(c.Do("GET", key))
	if err != nil {
		if err == redis.ErrNil {
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
func DeletePendingMACCommand(p *redis.Pool, devEUI lorawan.EUI64, cid lorawan.CID) error {
	c := p.Get()
	defer c.Close()

	key := fmt.Sprintf(macCommandPendingTempl, devEUI, cid)
	val, err := redis.Int(c.Do("DEL", key))
	if err != nil {
		return errors.Wrap(err, "delete pending mac-command error")
	}
	if val == 0 {
		return ErrDoesNotExist
	}

	log.WithFields(log.Fields{
		"dev_eui": devEUI,
		"cid":     cid,
	}).Info("pending mac-command deleted")

	return nil
}
