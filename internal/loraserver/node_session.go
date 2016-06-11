package loraserver

import (
	"bytes"
	"crypto/aes"
	"crypto/rand"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/brocaar/loraserver/models"
	"github.com/brocaar/lorawan"
	"github.com/garyburd/redigo/redis"
)

// NodeSession related constants
const (
	NodeSessionTTL = time.Hour * 24 * 5 // TTL of a node session (will be renewed on each activity)
)

const (
	nodeSessionKeyTempl        = "node_session_%s"
	nodeSessionMACTXQueueTempl = "node_session_mac_tx_queue_%s"
)

// createNodeSession does the same as saveNodeSession except that it does not
// overwrite an exisitng record.
func createNodeSession(p *redis.Pool, s models.NodeSession) error {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(s); err != nil {
		return fmt.Errorf("encode node-session for node %s error: %s", s.DevEUI, err)
	}

	c := p.Get()
	defer c.Close()

	exp := int64(NodeSessionTTL) / int64(time.Millisecond)

	if _, err := redis.String(c.Do("SET", fmt.Sprintf(nodeSessionKeyTempl, s.DevAddr), buf.Bytes(), "NX", "PX", exp)); err != nil {
		return fmt.Errorf("create node-session %s for node %s error: %s", s.DevAddr, s.DevEUI, err)
	}
	// DevEUI -> DevAddr pointer
	if _, err := redis.String(c.Do("PSETEX", fmt.Sprintf(nodeSessionKeyTempl, s.DevEUI), exp, s.DevAddr.String())); err != nil {
		return fmt.Errorf("create pointer node %s -> DevAddr %s error: %s", s.DevEUI, s.DevAddr, err)
	}

	log.WithField("dev_addr", s.DevAddr).Info("node-session created")
	return nil
}

// saveNodeSession saves the node session. Note that the session will automatically
// expire after NodeSessionTTL.
func saveNodeSession(p *redis.Pool, s models.NodeSession) error {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(s); err != nil {
		return fmt.Errorf("encode node-session for node %s error: %s", s.DevEUI, err)
	}

	c := p.Get()
	defer c.Close()

	exp := int64(NodeSessionTTL) / int64(time.Millisecond)

	if _, err := redis.String(c.Do("PSETEX", fmt.Sprintf(nodeSessionKeyTempl, s.DevAddr), exp, buf.Bytes())); err != nil {
		return fmt.Errorf("save node-session %s for node %s error: %s", s.DevAddr, s.DevEUI, err)
	}
	// DevEUI -> DevAddr pointer
	if _, err := redis.String(c.Do("PSETEX", fmt.Sprintf(nodeSessionKeyTempl, s.DevEUI), exp, s.DevAddr.String())); err != nil {
		return fmt.Errorf("create pointer node %s -> DevAddr %s error: %s", s.DevEUI, s.DevAddr, err)
	}

	log.WithField("dev_addr", s.DevAddr).Info("node-session saved")
	return nil
}

// getNodeSession returns the NodeSession for the given DevAddr.
func getNodeSession(p *redis.Pool, devAddr lorawan.DevAddr) (models.NodeSession, error) {
	var ns models.NodeSession

	c := p.Get()
	defer c.Close()

	val, err := redis.Bytes(c.Do("GET", fmt.Sprintf(nodeSessionKeyTempl, devAddr)))
	if err != nil {
		return ns, fmt.Errorf("get node-session for DevAddr %s error: %s", devAddr, err)
	}

	err = gob.NewDecoder(bytes.NewReader(val)).Decode(&ns)
	if err != nil {
		return ns, fmt.Errorf("decode node-session %s error: %s", devAddr, err)
	}

	return ns, nil
}

// getNodeSessionByDevEUI returns the NodeSession for the given DevEUI.
func getNodeSessionByDevEUI(p *redis.Pool, devEUI lorawan.EUI64) (models.NodeSession, error) {
	var ns models.NodeSession

	c := p.Get()
	defer c.Close()

	devAddr, err := redis.String(c.Do("GET", fmt.Sprintf(nodeSessionKeyTempl, devEUI)))
	if err != nil {
		return ns, fmt.Errorf("get node-session pointer for node %s error: %s", devEUI, err)
	}

	val, err := redis.Bytes(c.Do("GET", fmt.Sprintf(nodeSessionKeyTempl, devAddr)))
	if err != nil {
		return ns, fmt.Errorf("get node-session for DevAddr %s error: %s", devAddr, err)
	}

	err = gob.NewDecoder(bytes.NewReader(val)).Decode(&ns)
	if err != nil {
		return ns, fmt.Errorf("decode node-session %s error: %s", devAddr, err)
	}

	return ns, nil
}

// deleteNodeSession deletes the NodeSession matching the given DevAddr.
func deleteNodeSession(p *redis.Pool, devAddr lorawan.DevAddr) error {
	c := p.Get()
	defer c.Close()

	val, err := redis.Int(c.Do("DEL", fmt.Sprintf(nodeSessionKeyTempl, devAddr)))
	if err != nil {
		return fmt.Errorf("delete node-session %s error: %s", devAddr, err)
	}
	if val == 0 {
		return fmt.Errorf("node-session %s does not exist", devAddr)
	}
	log.WithField("dev_addr", devAddr).Info("node-session deleted")
	return nil
}

// getRandomDevAddr returns a random free DevAddr. Note that the 7 MSB will be
// set to the NwkID (based on the configured NetID).
// TODO: handle collission with retry?
func getRandomDevAddr(p *redis.Pool, netID lorawan.NetID) (lorawan.DevAddr, error) {
	var d lorawan.DevAddr
	b := make([]byte, len(d))
	if _, err := rand.Read(b); err != nil {
		return d, fmt.Errorf("could not read from random reader: %s", err)
	}
	copy(d[:], b)
	d[0] = d[0] & 1                    // zero out 7 msb
	d[0] = d[0] ^ (netID.NwkID() << 1) // set 7 msb to NwkID

	c := p.Get()
	defer c.Close()

	key := "node_session_" + d.String()
	val, err := redis.Int(c.Do("EXISTS", key))
	if err != nil {
		return lorawan.DevAddr{}, fmt.Errorf("test DevAddr %s exist error: %s", d, err)
	}
	if val == 1 {
		return lorawan.DevAddr{}, fmt.Errorf("DevAddr %s already exists", d)
	}
	return d, nil
}

// getAppNonce returns a random application nonce (used for OTAA).
func getAppNonce() ([3]byte, error) {
	var b [3]byte
	if _, err := rand.Read(b[:]); err != nil {
		return b, err
	}
	return b, nil
}

// addMACPayloadToTXQueue adds the given payload to the queue of MAC commands
// to send to the node. Note that the queue is bound to the node-session, since
// all mac operations are reset after a re-join of the node.
func addMACPayloadToTXQueue(p *redis.Pool, pl models.MACPayload) error {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(pl); err != nil {
		return fmt.Errorf("gob encode tx mac-payload for node %s error: %s", pl.DevEUI, err)
	}

	c := p.Get()
	defer c.Close()

	ns, err := getNodeSessionByDevEUI(p, pl.DevEUI)
	if err != nil {
		return fmt.Errorf("get node-session for node %s error: %s", pl.DevEUI, err)
	}

	exp := int64(NodeSessionTTL) / int64(time.Millisecond)
	key := fmt.Sprintf(nodeSessionMACTXQueueTempl, ns.DevAddr)

	c.Send("MULTI")
	c.Send("RPUSH", key, buf.Bytes())
	c.Send("PEXPIRE", key, exp)
	_, err = c.Do("EXEC")

	if err != nil {
		return fmt.Errorf("add mac-payload to tx queue for node %s error: %s", pl.DevEUI, err)
	}
	log.WithFields(log.Fields{
		"dev_eui":   pl.DevEUI,
		"dev_addr":  ns.DevAddr,
		"reference": pl.Reference,
	}).Info("mac-payload added to tx queue")
	return nil
}

// readMACPayloadTXQueue reads the full MACPayload tx queue for the given
// device address.
func readMACPayloadTXQueue(p *redis.Pool, devAddr lorawan.DevAddr) ([]models.MACPayload, error) {
	var out []models.MACPayload

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

		var pl models.MACPayload
		err = gob.NewDecoder(bytes.NewReader(b)).Decode(&pl)
		if err != nil {
			return nil, fmt.Errorf("decode mac-payload for devaddr %s error: %s", devAddr, err)
		}
		out = append(out, pl)
	}
	return out, nil
}

// deleteMACPayloadFromTXQueue deletes the given MACPayload from the tx queue
// of the given device address.
func deleteMACPayloadFromTXQueue(p *redis.Pool, devAddr lorawan.DevAddr, pl models.MACPayload) error {
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
		return fmt.Errorf("mac-payload with reference '%s' is not in tx queue for devaddr %s", pl.Reference, devAddr)
	}

	log.WithFields(log.Fields{
		"dev_eui":   pl.DevEUI,
		"dev_addr":  devAddr,
		"reference": pl.Reference,
	}).Info("mac-payload removed from tx queue")
	return nil
}

// getNwkSKey returns the network session key.
func getNwkSKey(appkey lorawan.AES128Key, netID lorawan.NetID, appNonce [3]byte, devNonce [2]byte) (lorawan.AES128Key, error) {
	return getSKey(0x01, appkey, netID, appNonce, devNonce)
}

// getAppSKey returns the application session key.
func getAppSKey(appkey lorawan.AES128Key, netID lorawan.NetID, appNonce [3]byte, devNonce [2]byte) (lorawan.AES128Key, error) {
	return getSKey(0x02, appkey, netID, appNonce, devNonce)
}

func getSKey(typ byte, appkey lorawan.AES128Key, netID lorawan.NetID, appNonce [3]byte, devNonce [2]byte) (lorawan.AES128Key, error) {
	var key lorawan.AES128Key
	b := make([]byte, 0, 16)
	b = append(b, typ)

	// little endian
	for i := len(appNonce) - 1; i >= 0; i-- {
		b = append(b, appNonce[i])
	}
	for i := len(netID) - 1; i >= 0; i-- {
		b = append(b, netID[i])
	}
	for i := len(devNonce) - 1; i >= 0; i-- {
		b = append(b, devNonce[i])
	}
	pad := make([]byte, 7)
	b = append(b, pad...)

	block, err := aes.NewCipher(appkey[:])
	if err != nil {
		return key, err
	}
	if block.BlockSize() != len(b) {
		return key, fmt.Errorf("block-size of %d bytes is expected", len(b))
	}
	block.Encrypt(key[:], b)
	return key, nil
}

// validateAndGetFullFCntUp validates if the given fCntUp is valid
// and returns the full 32 bit frame-counter.
// Note that the LoRaWAN packet only contains the 16 LSB, so in order
// to validate the MIC, the full 32 bit frame-counter needs to be set.
// After a succesful validation of the FCntUp and the MIC, don't forget
// to synchronize the Node FCntUp with the packet FCnt.
func validateAndGetFullFCntUp(n models.NodeSession, fCntUp uint32) (uint32, bool) {
	// we need to compare the difference of the 16 LSB
	gap := uint32(uint16(fCntUp) - uint16(n.FCntUp%65536))
	if gap < Band.MaxFCntGap {
		return n.FCntUp + gap, true
	}
	return 0, false
}

// NodeSessionAPI exports the NodeSession related functions.
type NodeSessionAPI struct {
	ctx Context
}

// NewNodeSessionAPI crestes a new NodeSessionAPI.
func NewNodeSessionAPI(ctx Context) *NodeSessionAPI {
	return &NodeSessionAPI{
		ctx: ctx,
	}
}

// Get returns the NodeSession for the given DevAddr.
func (a *NodeSessionAPI) Get(devAddr lorawan.DevAddr, ns *models.NodeSession) error {
	var err error
	*ns, err = getNodeSession(a.ctx.RedisPool, devAddr)
	return err
}

// GetByDevEUI returns the NodeSession for the given DevEUI.
func (a *NodeSessionAPI) GetByDevEUI(devEUI lorawan.EUI64, ns *models.NodeSession) error {
	var err error
	*ns, err = getNodeSessionByDevEUI(a.ctx.RedisPool, devEUI)
	return err
}

// Create creates the given NodeSession (activation by personalization).
// The DevAddr must contain the same NwkID as the configured NetID.
// Sessions will expire automatically after the configured TTL.
func (a *NodeSessionAPI) Create(ns models.NodeSession, devAddr *lorawan.DevAddr) error {
	// validate the NwkID
	if ns.DevAddr.NwkID() != a.ctx.NetID.NwkID() {
		return fmt.Errorf("DevAddr must contain NwkID %s", hex.EncodeToString([]byte{a.ctx.NetID.NwkID()}))
	}

	// validate that the node exists
	var node models.Node
	var err error
	if node, err = getNode(a.ctx.DB, ns.DevEUI); err != nil {
		return err
	}

	if ns.AppEUI != node.AppEUI {
		return fmt.Errorf("DevEUI %s belongs to AppEUI %s, got AppEUI %s", ns.AppEUI, node.AppEUI, ns.AppEUI)
	}

	if err = createNodeSession(a.ctx.RedisPool, ns); err != nil {
		return err
	}
	*devAddr = ns.DevAddr
	return nil
}

// Update updates the given NodeSession.
func (a *NodeSessionAPI) Update(ns models.NodeSession, devEUI *lorawan.EUI64) error {
	// validate the NwkID
	if ns.DevAddr.NwkID() != a.ctx.NetID.NwkID() {
		return fmt.Errorf("DevAddr must contain NwkID %s", hex.EncodeToString([]byte{a.ctx.NetID.NwkID()}))
	}

	if err := saveNodeSession(a.ctx.RedisPool, ns); err != nil {
		return err
	}
	*devEUI = ns.DevEUI
	return nil
}

// Delete the NodeSession matching the given DevAddr.
func (a *NodeSessionAPI) Delete(devAddr lorawan.DevAddr, deletedDevAddr *lorawan.DevAddr) error {
	if err := deleteNodeSession(a.ctx.RedisPool, devAddr); err != nil {
		return err
	}
	*deletedDevAddr = devAddr
	return nil
}

// GetRandomDevAddr returns a random DevAddr.
func (a *NodeSessionAPI) GetRandomDevAddr(dummy interface{}, devAddr *lorawan.DevAddr) error {
	var err error
	*devAddr, err = getRandomDevAddr(a.ctx.RedisPool, a.ctx.NetID)
	return err
}
