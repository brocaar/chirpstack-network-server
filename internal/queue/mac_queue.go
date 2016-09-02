package queue

import (
	"bytes"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/garyburd/redigo/redis"

	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/session"
	"github.com/brocaar/lorawan"
)

// AddMACPayloadToTXQueue adds the given payload to the queue of MAC commands
// to send to the node. Note that the queue is bound to the node-session, since
// all mac operations are reset after a re-join of the node.
func AddMACPayloadToTXQueue(p *redis.Pool, pl MACPayload) error {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(pl); err != nil {
		return fmt.Errorf("gob encode tx mac-payload for node %s error: %s", pl.DevEUI, err)
	}

	c := p.Get()
	defer c.Close()

	ns, err := session.GetNodeSessionByDevEUI(p, pl.DevEUI)
	if err != nil {
		return fmt.Errorf("get node-session for node %s error: %s", pl.DevEUI, err)
	}

	exp := int64(common.NodeSessionTTL) / int64(time.Millisecond)
	key := fmt.Sprintf(nodeSessionMACTXQueueTempl, ns.DevAddr)

	c.Send("MULTI")
	c.Send("RPUSH", key, buf.Bytes())
	c.Send("PEXPIRE", key, exp)
	_, err = c.Do("EXEC")

	if err != nil {
		return fmt.Errorf("add mac-payload to tx queue for node %s error: %s", pl.DevEUI, err)
	}
	log.WithFields(log.Fields{
		"dev_eui":    pl.DevEUI,
		"dev_addr":   ns.DevAddr,
		"frmpayload": pl.FRMPayload,
		"command":    hex.EncodeToString(pl.Data),
	}).Info("mac-payload added to tx queue")
	return nil
}

const (
	nodeSessionMACTXQueueTempl = "node_session_mac_tx_queue_%s"
)

// ReadMACPayloadTXQueue reads the full MACPayload tx queue for the given
// device address.
func ReadMACPayloadTXQueue(p *redis.Pool, devAddr lorawan.DevAddr) ([]MACPayload, error) {
	var out []MACPayload

	c := p.Get()
	defer c.Close()

	key := fmt.Sprintf(nodeSessionMACTXQueueTempl, devAddr)
	values, err := redis.Values(c.Do("LRANGE", key, 0, -1))
	if err != nil {
		return nil, fmt.Errorf("get mac-payload from tx queue for devaddr %s error: %s", devAddr, err)
	}

	for _, value := range values {
		b, ok := value.([]byte)
		if !ok {
			return nil, fmt.Errorf("expected []byte type, got %T", value)
		}

		var pl MACPayload
		err = gob.NewDecoder(bytes.NewReader(b)).Decode(&pl)
		if err != nil {
			return nil, fmt.Errorf("decode mac-payload for devaddr %s error: %s", devAddr, err)
		}
		out = append(out, pl)
	}
	return out, nil
}

// FilterMACPayloads filters the given slice of MACPayload elements based
// on the given criteria (FRMPayload and max-bytes).
func FilterMACPayloads(payloads []MACPayload, frmPayload bool, maxBytes int) []MACPayload {
	var out []MACPayload
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

// DeleteMACPayloadFromTXQueue deletes the given MACPayload from the tx queue
// of the given device address.
func DeleteMACPayloadFromTXQueue(p *redis.Pool, devAddr lorawan.DevAddr, pl MACPayload) error {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(pl); err != nil {
		return fmt.Errorf("gob encode tx mac-payload for node %s error: %s", pl.DevEUI, err)
	}

	c := p.Get()
	defer c.Close()

	key := fmt.Sprintf(nodeSessionMACTXQueueTempl, devAddr)
	val, err := redis.Int(c.Do("LREM", key, 0, buf.Bytes()))
	if err != nil {
		return fmt.Errorf("delete mac-payload from tx queue for devaddr %s error: %s", devAddr, err)
	}

	if val == 0 {
		return fmt.Errorf("mac-command %X not in tx queue for devaddr %s", pl.Data, devAddr)
	}

	log.WithFields(log.Fields{
		"dev_eui":  pl.DevEUI,
		"dev_addr": devAddr,
		"command":  hex.EncodeToString(pl.Data),
	}).Info("mac-payload removed from tx queue")
	return nil
}
