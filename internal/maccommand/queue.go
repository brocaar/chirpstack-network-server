package maccommand

import (
	"bytes"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/garyburd/redigo/redis"

	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/lorawan"
)

const (
	queueTempl   = "lora:ns:mac:queue:%s"
	pendingTempl = "lora:ns:mac:pending:%s:%d"
)

// AddToQueue adds the given payload to the queue of MAC commands
// to send to the node.
func AddToQueue(p *redis.Pool, pl QueueItem) error {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(pl); err != nil {
		return fmt.Errorf("gob encode tx mac-payload for node %s error: %s", pl.DevEUI, err)
	}

	c := p.Get()
	defer c.Close()

	exp := int64(common.NodeSessionTTL) / int64(time.Millisecond)
	key := fmt.Sprintf(queueTempl, pl.DevEUI)

	c.Send("MULTI")
	c.Send("RPUSH", key, buf.Bytes())
	c.Send("PEXPIRE", key, exp)
	_, err := c.Do("EXEC")

	if err != nil {
		return fmt.Errorf("add mac-payload to tx queue for node %s error: %s", pl.DevEUI, err)
	}
	log.WithFields(log.Fields{
		"dev_eui":    pl.DevEUI,
		"frmpayload": pl.FRMPayload,
		"command":    hex.EncodeToString(pl.Data),
	}).Info("mac-payload added to tx queue")
	return nil
}

// ReadQueue reads the full mac-payload queue for the given devEUI.
func ReadQueue(p *redis.Pool, devEUI lorawan.EUI64) ([]QueueItem, error) {
	var out []QueueItem

	c := p.Get()
	defer c.Close()

	key := fmt.Sprintf(queueTempl, devEUI)
	values, err := redis.Values(c.Do("LRANGE", key, 0, -1))
	if err != nil {
		return nil, fmt.Errorf("get mac-payload from tx queue for deveui %s error: %s", devEUI, err)
	}

	for _, value := range values {
		b, ok := value.([]byte)
		if !ok {
			return nil, fmt.Errorf("expected []byte type, got %T", value)
		}

		var pl QueueItem
		err = gob.NewDecoder(bytes.NewReader(b)).Decode(&pl)
		if err != nil {
			return nil, fmt.Errorf("decode mac-payload for deveui %s error: %s", devEUI, err)
		}
		out = append(out, pl)
	}
	return out, nil
}

// FlushQueue flushes the mac-payload queue for the given devEUI.
func FlushQueue(p *redis.Pool, devEUI lorawan.EUI64) error {
	c := p.Get()
	defer c.Close()

	key := fmt.Sprintf(queueTempl, devEUI)
	_, err := redis.Int(c.Do("DEL", key))
	if err != nil {
		return fmt.Errorf("flush queue error: %s", err)
	}
	return nil
}

// FilterItems filters the given slice of MACPayload elements based
// on the given criteria (FRMPayload and max-bytes).
func FilterItems(payloads []QueueItem, frmPayload bool, maxBytes int) []QueueItem {
	var out []QueueItem
	var byteCount int
	for _, pl := range payloads {
		if pl.FRMPayload == frmPayload {
			byteCount += len(pl.Data)
			if byteCount > maxBytes {
				return out
			}
			out = append(out, pl)
		}
	}
	return out
}

// DeleteQueueItem deletes the given mac-command from the tx queue
// of the given device address.
func DeleteQueueItem(p *redis.Pool, devEUI lorawan.EUI64, pl QueueItem) error {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(pl); err != nil {
		return fmt.Errorf("gob encode tx mac-payload for node %s error: %s", pl.DevEUI, err)
	}

	c := p.Get()
	defer c.Close()

	key := fmt.Sprintf(queueTempl, devEUI)
	val, err := redis.Int(c.Do("LREM", key, 0, buf.Bytes()))
	if err != nil {
		return fmt.Errorf("delete mac-payload from tx queue for deveui %s error: %s", devEUI, err)
	}

	if val == 0 {
		return fmt.Errorf("mac-command %X not in tx queue for deveui %s", pl.Data, devEUI)
	}

	log.WithFields(log.Fields{
		"dev_eui": pl.DevEUI,
		"command": hex.EncodeToString(pl.Data),
	}).Info("mac-payload removed from tx queue")
	return nil
}

// SetPending sets one or multiple MACCommandPayload to the pending buffer.
// It overwrites existing payloads for the given CID.
func SetPending(p *redis.Pool, devEUI lorawan.EUI64, cid lorawan.CID, payloads []lorawan.MACCommandPayload) error {
	c := p.Get()
	defer c.Close()

	key := fmt.Sprintf(pendingTempl, devEUI, cid)
	exp := int64(common.MACPendingTTL) / int64(time.Millisecond)

	c.Send("MULTI")
	c.Send("DEL", key)
	for _, pl := range payloads {
		b, err := pl.MarshalBinary()
		if err != nil {
			return fmt.Errorf("marshal mac-payload error: %s", err)
		}
		c.Send("RPUSH", key, b)
	}
	c.Send("PEXPIRE", key, exp)

	if _, err := c.Do("EXEC"); err != nil {
		return fmt.Errorf("write mac-commands to pending error: %s", err)
	}

	return nil
}

// ReadPending returns the pending MACCommandPayload items for the given CID.
// In case no items are pending, an empty slice is returned.
func ReadPending(p *redis.Pool, devEUI lorawan.EUI64, cid lorawan.CID) ([]lorawan.MACCommandPayload, error) {
	var out []lorawan.MACCommandPayload
	c := p.Get()
	defer c.Close()

	key := fmt.Sprintf(pendingTempl, devEUI, cid)
	values, err := redis.Values(c.Do("LRANGE", key, 0, -1))
	if err != nil {
		return nil, fmt.Errorf("get pending mac-commands for DevEUI %s and CID %d error: %s", devEUI, cid, err)
	}

	for _, value := range values {
		b, ok := value.([]byte)
		if !ok {
			return nil, fmt.Errorf("expected []byte type, got %T", value)
		}

		pl, _, err := lorawan.GetMACPayloadAndSize(false, cid)
		if err != nil {
			return nil, fmt.Errorf("get mac-payload error: %s", err)
		}

		if err := pl.UnmarshalBinary(b); err != nil {
			return nil, fmt.Errorf("unmarshal mac-payload error: %s", err)
		}

		out = append(out, pl)
	}

	return out, nil
}
