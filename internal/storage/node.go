package storage

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/garyburd/redigo/redis"
	"github.com/jmoiron/sqlx"

	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/models"
	"github.com/brocaar/lorawan"
)

const (
	nodeTXPayloadQueueTempl     = "node_tx_queue_%s"
	nodeTXPayloadInProcessTempl = "node_tx_in_process_%s"
)

// CreateNode creates the given Node.
func CreateNode(db *sqlx.DB, n models.Node) error {
	if n.RXDelay > 15 {
		return errors.New("max value of RXDelay is 15")
	}

	_, err := db.Exec(`
		insert into node (
			dev_eui,
			app_eui,
			app_key,
			rx_delay,
			rx1_dr_offset,
			channel_list_id
		)
		values ($1, $2, $3, $4, $5, $6)`,
		n.DevEUI[:],
		n.AppEUI[:],
		n.AppKey[:],
		n.RXDelay,
		n.RX1DROffset,
		n.ChannelListID,
	)
	if err != nil {
		return fmt.Errorf("create node %s error: %s", n.DevEUI, err)
	}
	log.WithField("dev_eui", n.DevEUI).Info("node created")
	return nil
}

// UpdateNode updates the given Node.
func UpdateNode(db *sqlx.DB, n models.Node) error {
	if n.RXDelay > 15 {
		return errors.New("max value of RXDelay is 15")
	}

	res, err := db.Exec(`
		update node set
			app_eui = $2,
			app_key = $3,
			used_dev_nonces = $4,
			rx_delay = $5,
			rx1_dr_offset = $6,
			channel_list_id = $7
		where dev_eui = $1`,
		n.DevEUI[:],
		n.AppEUI[:],
		n.AppKey[:],
		n.UsedDevNonces,
		n.RXDelay,
		n.RX1DROffset,
		n.ChannelListID,
	)
	if err != nil {
		return fmt.Errorf("update node %s error: %s", n.DevEUI, err)
	}
	ra, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if ra == 0 {
		return fmt.Errorf("node %s does not exist", n.DevEUI)
	}
	log.WithField("dev_eui", n.DevEUI).Info("node updated")
	return nil
}

// DeleteNode deletes the Node matching the given DevEUI.
func DeleteNode(db *sqlx.DB, p *redis.Pool, devEUI lorawan.EUI64) error {
	ns, err := GetNodeSessionByDevEUI(p, devEUI)
	if err == nil {
		if err = DeleteNodeSession(p, ns.DevAddr); err != nil {
			return err
		}
	}

	res, err := db.Exec("delete from node where dev_eui = $1",
		devEUI[:],
	)
	if err != nil {
		return fmt.Errorf("delete node %s error: %s", devEUI, err)
	}
	ra, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if ra == 0 {
		return fmt.Errorf("node %s does not exist", devEUI)
	}
	log.WithField("dev_eui", devEUI).Info("node deleted")
	return nil
}

// GetNode returns the Node for the given DevEUI.
func GetNode(db *sqlx.DB, devEUI lorawan.EUI64) (models.Node, error) {
	var node models.Node
	err := db.Get(&node, "select * from node where dev_eui = $1", devEUI[:])
	if err != nil {
		return node, fmt.Errorf("get node %s error: %s", devEUI, err)
	}
	return node, nil
}

// GetNodesCount returns the total number of nodes.
func GetNodesCount(db *sqlx.DB) (int, error) {
	var count struct {
		Count int
	}
	err := db.Get(&count, "select count(*) as count from node")
	if err != nil {
		return 0, fmt.Errorf("get nodes count error: %s", err)
	}
	return count.Count, nil
}

// GetNodesForAppEUICount returns the total number of nodes given an AppEUI.
func GetNodesForAppEUICount(db *sqlx.DB, appEUI lorawan.EUI64) (int, error) {
	var count struct {
		Count int
	}
	err := db.Get(&count, "select count(*) as count from node where app_eui = $1", appEUI[:])
	if err != nil {
		return 0, fmt.Errorf("get nodes count for app_eui=%s error: %s", appEUI, err)
	}
	return count.Count, nil
}

// GetNodes returns a slice of nodes, sorted by DevEUI.
func GetNodes(db *sqlx.DB, limit, offset int) ([]models.Node, error) {
	var nodes []models.Node
	err := db.Select(&nodes, "select * from node order by dev_eui limit $1 offset $2", limit, offset)
	if err != nil {
		return nodes, fmt.Errorf("get nodes error: %s", err)
	}
	return nodes, nil
}

// GetNodesForAppEUI returns a slice of nodes, sorted by DevEUI, for the given AppEUI.
func GetNodesForAppEUI(db *sqlx.DB, appEUI lorawan.EUI64, limit, offset int) ([]models.Node, error) {
	var nodes []models.Node
	err := db.Select(&nodes, "select * from node where app_eui = $1 order by dev_eui limit $2 offset $3", appEUI[:], limit, offset)
	if err != nil {
		return nodes, fmt.Errorf("get nodes error: %s", err)
	}
	return nodes, nil
}

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

// GetCFListForNode returns the CFList for the given node if the
// used ISM band allows using a CFList.
func GetCFListForNode(db *sqlx.DB, node models.Node) (*lorawan.CFList, error) {
	if node.ChannelListID == nil {
		return nil, nil
	}

	if !common.Band.ImplementsCFlist {
		log.WithFields(log.Fields{
			"dev_eui": node.DevEUI,
			"app_eui": node.AppEUI,
		}).Warning("node has channel-list, but CFList not allowed for selected band")
		return nil, nil
	}

	channels, err := GetChannelsForChannelList(db, *node.ChannelListID)
	if err != nil {
		return nil, err
	}

	var cFList lorawan.CFList
	for _, channel := range channels {
		if len(cFList) <= channel.Channel-3 {
			return nil, fmt.Errorf("invalid channel index for CFList: %d", channel.Channel)
		}
		cFList[channel.Channel-3] = uint32(channel.Frequency)
	}
	return &cFList, nil
}
