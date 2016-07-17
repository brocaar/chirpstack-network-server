package loraserver

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/brocaar/loraserver/models"
	"github.com/brocaar/lorawan"
	"github.com/garyburd/redigo/redis"
	"github.com/jmoiron/sqlx"
)

// NodeTXPayloadQueueTTL defines the TTL of the node TXPayload queue
var NodeTXPayloadQueueTTL = time.Hour * 24 * 5

// errEmptyQueue defines the error returned when the queue is empty.
// Note that depending the context, this error might not be a real error.
var errEmptyQueue = errors.New("the queue is empty or does not exist")

const (
	nodeTXPayloadQueueTempl     = "node_tx_queue_%s"
	nodeTXPayloadInProcessTempl = "node_tx_in_process_%s"
)

// createNode creates the given Node.
func createNode(db *sqlx.DB, n models.Node) error {
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

// updateNode updates the given Node.
func updateNode(db *sqlx.DB, n models.Node) error {
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

// deleteNode deletes the Node matching the given DevEUI.
func deleteNode(db *sqlx.DB, devEUI lorawan.EUI64) error {
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

// getNode returns the Node for the given DevEUI.
func getNode(db *sqlx.DB, devEUI lorawan.EUI64) (models.Node, error) {
	var node models.Node
	err := db.Get(&node, "select * from node where dev_eui = $1", devEUI[:])
	if err != nil {
		return node, fmt.Errorf("get node %s error: %s", devEUI, err)
	}
	return node, nil
}

// getNodes returns a slice of nodes, sorted by DevEUI.
func getNodes(db *sqlx.DB, limit, offset int) ([]models.Node, error) {
	var nodes []models.Node
	err := db.Select(&nodes, "select * from node order by dev_eui limit $1 offset $2", limit, offset)
	if err != nil {
		return nodes, fmt.Errorf("get nodes error: %s", err)
	}
	return nodes, nil
}

// getNodesForAppEUI returns a slice of nodes, sorted by DevEUI, for the given AppEUI.
func getNodesForAppEUI(db *sqlx.DB, appEUI lorawan.EUI64, limit, offset int) ([]models.Node, error) {
	var nodes []models.Node
	err := db.Select(&nodes, "select * from node where app_eui = $1 order by dev_eui limit $2 offset $3", appEUI[:], limit, offset)
	if err != nil {
		return nodes, fmt.Errorf("get nodes error: %s", err)
	}
	return nodes, nil
}

// addTXPayloadToQueue adds the given TXPayload to the queue.
func addTXPayloadToQueue(p *redis.Pool, payload models.TXPayload) error {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(payload); err != nil {
		return fmt.Errorf("encode tx-payload for node %s error: %s", payload.DevEUI, err)
	}

	c := p.Get()
	defer c.Close()

	exp := int64(NodeTXPayloadQueueTTL) / int64(time.Millisecond)
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

// getTXPayloadQueueSize returns the total TXPayload elements in the queue
// (including the in-process queue).
func getTXPayloadQueueSize(p *redis.Pool, devEUI lorawan.EUI64) (int, error) {
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

// getTXPayloadFromQueue returns the first TXPayload to send to the node.
// The TXPayload either is a payload that is still in-process (e.g. a payload
// that needs to be re-transmitted) or an item from the queue.
// After a successful transmission, don't forget to call
// clearInProcessTXPayload.
// errEmptyQueue is returned when the queue is empty / does not exist.
func getTXPayloadFromQueue(p *redis.Pool, devEUI lorawan.EUI64) (models.TXPayload, error) {
	var txPayload models.TXPayload
	queueKey := fmt.Sprintf(nodeTXPayloadQueueTempl, devEUI)
	inProcessKey := fmt.Sprintf(nodeTXPayloadInProcessTempl, devEUI)
	exp := int64(NodeTXPayloadQueueTTL) / int64(time.Millisecond)

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
			return txPayload, errEmptyQueue
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

// clearInProcessTXPayload clears the in-process TXPayload (to be called
// after a successful transmission). It returns the TXPayload or nil when
// nothing was cleared (it already expired).
func clearInProcessTXPayload(p *redis.Pool, devEUI lorawan.EUI64) (*models.TXPayload, error) {
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

// flushTXPayloadQueue flushes the tx payload queue for the given DevEUI.
func flushTXPayloadQueue(p *redis.Pool, devEUI lorawan.EUI64) error {
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

// getCFListForNode returns the CFList for the given node if the
// used ISM band allows using a CFList.
func getCFListForNode(db *sqlx.DB, node models.Node) (*lorawan.CFList, error) {
	if node.ChannelListID == nil {
		return nil, nil
	}

	if !Band.ImplementsCFlist {
		log.WithFields(log.Fields{
			"dev_eui": node.DevEUI,
			"app_eui": node.AppEUI,
		}).Warning("node has channel-list, but CFList not allowed for selected band")
		return nil, nil
	}

	channels, err := getChannelsForChannelList(db, *node.ChannelListID)
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

// NodeAPI exports the Node related functions.
type NodeAPI struct {
	ctx Context
}

// NewNodeAPI creates a new NodeAPI.
func NewNodeAPI(ctx Context) *NodeAPI {
	return &NodeAPI{
		ctx: ctx,
	}
}

// Get returns the Node for the given DevEUI.
func (a *NodeAPI) Get(devEUI lorawan.EUI64, node *models.Node) error {
	var err error
	*node, err = getNode(a.ctx.DB, devEUI)
	return err
}

// GetList returns a list of nodes (given a limit and offset).
func (a *NodeAPI) GetList(req models.GetListRequest, nodes *[]models.Node) error {
	var err error
	*nodes, err = getNodes(a.ctx.DB, req.Limit, req.Offset)
	return err
}

// GetListForAppEUI returns a list of nodes (given an AppEUI, limit and offset).
func (a *NodeAPI) GetListForAppEUI(req models.GetListForAppEUIRequest, nodes *[]models.Node) error {
	var err error
	*nodes, err = getNodesForAppEUI(a.ctx.DB, req.AppEUI, req.Limit, req.Offset)
	return err
}

// Create creates the given Node.
func (a *NodeAPI) Create(node models.Node, devEUI *lorawan.EUI64) error {
	if err := createNode(a.ctx.DB, node); err != nil {
		return err
	}
	*devEUI = node.DevEUI
	return nil
}

// Update updatest the given Node.
func (a *NodeAPI) Update(node models.Node, devEUI *lorawan.EUI64) error {
	if err := updateNode(a.ctx.DB, node); err != nil {
		return err
	}
	*devEUI = node.DevEUI
	return nil
}

// Delete deletes the node matching the given DevEUI.
func (a *NodeAPI) Delete(devEUI lorawan.EUI64, deletedDevEUI *lorawan.EUI64) error {
	if err := deleteNode(a.ctx.DB, devEUI); err != nil {
		return err
	}
	*deletedDevEUI = devEUI
	return nil
}

// FlushTXPayloadQueue flushes the tx-payload queue for the given DevEUI.
func (a *NodeAPI) FlushTXPayloadQueue(devEUI lorawan.EUI64, affectedDevEUI *lorawan.EUI64) error {
	if err := flushTXPayloadQueue(a.ctx.RedisPool, devEUI); err != nil {
		return err
	}
	*affectedDevEUI = devEUI
	return nil
}
