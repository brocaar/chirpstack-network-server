package loraserver

import (
	"bytes"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/brocaar/loraserver/models"
	"github.com/garyburd/redigo/redis"
)

// Packet collection constants
const (
	CollectAndCallOnceWait = time.Millisecond * 100 // the time to wait for the same packet received by multiple gateways
	CollectDataDownWait    = time.Millisecond * 100 // the time to wait on possible downlink payloads from the application
)

// RXPackets is a slice of RXPacket. It implements sort.Interface
// to sort the slice of packets by signal strength so that the
// packet received with the strongest signal will be at index 0
// of RXPackets.
type RXPackets []models.RXPacket

// Len is part of sort.Interface.
func (p RXPackets) Len() int {
	return len(p)
}

// Swap is part of sort.Interface.
func (p RXPackets) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}

// Less is part of sort.Interface.
func (p RXPackets) Less(i, j int) bool {
	return p[i].RXInfo.RSSI > p[j].RXInfo.RSSI
}

// collectAndCallOnce collects the package, sleeps the configured duraction and
// calls the callback only once with a slice of packets, sorted by signal
// strength (strongest at index 0). This method exists since multiple gateways
// are able to receive the same packet, but the packet needs to processed
// only once.
// It is important to validate the packet before calling collectAndCallOnce
// (since the MIC is part of the storage key, make sure it is valid).
// It is safe to collect the same packet received by the same gateway twice.
// Since the underlying storage type is a set, the result will always be a
// unique set per gateway MAC and packet MIC.
func collectAndCallOnce(p *redis.Pool, rxPacket models.RXPacket, callback func(packets RXPackets) error) error {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(rxPacket); err != nil {
		return fmt.Errorf("encode rx packet error: %s", err)
	}
	c := p.Get()
	defer c.Close()

	// store the packet in a set with CollectAndCallOnceWait expiration
	// in case the packet is received by multiple gateways, the set will contain
	// each packet.
	key := "collect_" + hex.EncodeToString(rxPacket.PHYPayload.MIC[:])
	c.Send("MULTI")
	c.Send("SADD", key, buf.Bytes())
	c.Send("PEXPIRE", key, int64(CollectAndCallOnceWait*2)/int64(time.Millisecond))
	_, err := c.Do("EXEC")
	if err != nil {
		return fmt.Errorf("add rx packet to collect set error: %s", err)
	}

	// acquire a lock on processing this packet
	_, err = redis.String((c.Do("SET", key+"_lock", "lock", "PX", int64(CollectAndCallOnceWait*2)/int64(time.Millisecond), "NX")))
	if err != nil {
		if err == redis.ErrNil {
			// the packet processing is already locked by an other process
			// so there is nothing to do anymore :-)
			return nil
		}
		return fmt.Errorf("acquire lock error: %s", err)
	}

	// wait the configured amount of time, more packets might be received
	// from other gateways
	time.Sleep(CollectAndCallOnceWait)

	// collect all packets from the set
	rxPackets := make(RXPackets, 0)
	payloads, err := redis.ByteSlices(c.Do("SMEMBERS", key))
	if err != nil {
		return fmt.Errorf("get collect set members error: %s", err)
	}
	if len(payloads) == 0 {
		return errors.New("zero items in collect set")
	}

	for _, b := range payloads {
		var packet models.RXPacket
		if err := gob.NewDecoder(bytes.NewReader(b)).Decode(&packet); err != nil {
			return fmt.Errorf("decode rx packet error: %s", err)
		}
		rxPackets = append(rxPackets, packet)
	}

	sort.Sort(rxPackets)
	return callback(rxPackets)
}
