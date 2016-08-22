package storage

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/garyburd/redigo/redis"

	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/models"
	"github.com/brocaar/lorawan"
)

const (
	nodeTXPayloadQueueTempl     = "node_tx_queue_%s"
	nodeTXPayloadInProcessTempl = "node_tx_in_process_%s"
)

// AddTXPayloadToQueue adds the given TXPayload to the queue.
func AddTXPayloadToQueue(p *redis.Pool, payload models.TXPayload) error {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(payload); err != nil {
		return fmt.Errorf("encode tx-payload for node %s error: %s", payload.DevEUI, err)
	}

	c := p.Get()
	defer c.Close()

	exp := int64(common.NodeTXPayloadQueueTTL) / int64(time.Millisecond)
	key := fmt.Sprintf(nodeTXPayloadQueueTempl, payload.DevEUI)

	c.Send("MULTI")
	c.Send("LPUSH", key, buf.Bytes())
	c.Send("PEXPIRE", key, exp)
	_, err := c.Do("EXEC")

	if err != nil {
		return fmt.Errorf("add tx-payload to queue for node %s error: %s", payload.DevEUI, err)
	}

	log.WithFields(log.Fields{
		"dev_eui":   payload.DevEUI,
		"reference": payload.Reference,
	}).Info("tx-payload added to queue")
	return nil
}

// GetTXPayloadQueueSize returns the total TXPayload elements in the queue
// (including the in-process queue).
func GetTXPayloadQueueSize(p *redis.Pool, devEUI lorawan.EUI64) (int, error) {
	var count int
	c := p.Get()
	defer c.Close()

	i, err := redis.Int(c.Do("LLEN", fmt.Sprintf(nodeTXPayloadInProcessTempl, devEUI)))
	if err != nil {
		return 0, fmt.Errorf("get in-process tx-payload queue length error: %s", err)
	}
	count += i

	i, err = redis.Int(c.Do("LLEN", fmt.Sprintf(nodeTXPayloadQueueTempl, devEUI)))
	if err != nil {
		return 0, fmt.Errorf("get tx-payload queue length error: %s", err)
	}
	count += i
	return count, nil
}

// GetTXPayloadFromQueue returns the first TXPayload to send to the node.
// The TXPayload either is a payload that is still in-process (e.g. a payload
// that needs to be re-transmitted) or an item from the queue.
// After a successful transmission, don't forget to call
// clearInProcessTXPayload.
// errEmptyQueue is returned when the queue is empty / does not exist.
func GetTXPayloadFromQueue(p *redis.Pool, devEUI lorawan.EUI64) (models.TXPayload, error) {
	var txPayload models.TXPayload
	queueKey := fmt.Sprintf(nodeTXPayloadQueueTempl, devEUI)
	inProcessKey := fmt.Sprintf(nodeTXPayloadInProcessTempl, devEUI)
	exp := int64(common.NodeTXPayloadQueueTTL) / int64(time.Millisecond)

	c := p.Get()
	defer c.Close()

	// in-process
	b, err := redis.Bytes(c.Do("LINDEX", inProcessKey, -1))
	if err != nil {
		if err != redis.ErrNil {
			return txPayload, fmt.Errorf("get tx-payload from in-process error: %s", err)
		}

		// in-process is empty, read from queue
		b, err = redis.Bytes(c.Do("RPOPLPUSH", queueKey, inProcessKey))
		if err != nil {
			if err != redis.ErrNil {
				return txPayload, fmt.Errorf("get tx-payload from queue error: %s", err)
			}
			return txPayload, common.ErrEmptyQueue
		}
		_, err = redis.Int(c.Do("PEXPIRE", inProcessKey, exp))
		if err != nil {
			return txPayload, fmt.Errorf("set expire on %s error: %s", inProcessKey, err)
		}
	}

	if err = gob.NewDecoder(bytes.NewReader(b)).Decode(&txPayload); err != nil {
		return txPayload, fmt.Errorf("decode tx-payload for node %s error: %s", devEUI, err)
	}

	return txPayload, nil
}

// ClearInProcessTXPayload clears the in-process TXPayload (to be called
// after a successful transmission). It returns the TXPayload or nil when
// nothing was cleared (it already expired).
func ClearInProcessTXPayload(p *redis.Pool, devEUI lorawan.EUI64) (*models.TXPayload, error) {
	var txPayload models.TXPayload
	key := fmt.Sprintf(nodeTXPayloadInProcessTempl, devEUI)
	c := p.Get()
	defer c.Close()
	b, err := redis.Bytes(c.Do("RPOP", key))
	if err != nil {
		if err == redis.ErrNil {
			return nil, nil
		}
		return nil, fmt.Errorf("clear in-process tx payload for node %s failed: %s", devEUI, err)
	}

	err = gob.NewDecoder(bytes.NewReader(b)).Decode(&txPayload)
	if err != nil {
		return nil, fmt.Errorf("decode tx-payload for node %s error: %s", devEUI, err)
	}

	log.WithFields(log.Fields{
		"dev_eui":   devEUI,
		"reference": txPayload.Reference,
	}).Info("in-process tx payload removed")
	return &txPayload, nil
}

// FlushTXPayloadQueue flushes the tx payload queue for the given DevEUI.
func FlushTXPayloadQueue(p *redis.Pool, devEUI lorawan.EUI64) error {
	keys := []interface{}{
		fmt.Sprintf(nodeTXPayloadInProcessTempl, devEUI),
		fmt.Sprintf(nodeTXPayloadQueueTempl, devEUI),
	}

	c := p.Get()
	defer c.Close()

	_, err := redis.Int(c.Do("DEL", keys...))
	if err != nil {
		return fmt.Errorf("flush tx-payload queue for DevEUI %s error: %s", devEUI, err)
	}
	log.WithFields(log.Fields{
		"dev_eui": devEUI,
	}).Info("tx-payload queue flushed")
	return nil
}
