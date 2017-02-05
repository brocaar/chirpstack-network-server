package uplink

import (
	"bytes"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/lorawan"
	"github.com/garyburd/redigo/redis"
)

// Templates used for generating Redis keys
const (
	CollectKeyTempl     = "loraserver:rx:collect:%s"
	CollectLockKeyTempl = "loraserver:rx:collect:%s:lock"
)

// collectAndCallOnce collects the package, sleeps the configured duraction and
// calls the callback only once with a slice of packets, sorted by signal
// strength (strongest at index 0). This method exists since multiple gateways
// are able to receive the same packet, but the packet needs to processed
// only once.
// It is safe to collect the same packet received by the same gateway twice.
// Since the underlying storage type is a set, the result will always be a
// unique set per gateway MAC and packet MIC.
func collectAndCallOnce(p *redis.Pool, rxPacket gw.RXPacket, callback func(packet models.RXPacket) error) error {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(rxPacket); err != nil {
		return fmt.Errorf("encode rx packet error: %s", err)
	}
	c := p.Get()
	defer c.Close()

	// store the packet in a set with DeduplicationDelay expiration
	// in case the packet is received by multiple gateways, the set will contain
	// each packet.
	// since we can't trust the MIC (in case of a join-request, it will be
	// validated after the collect), we generate a new one and use it as the
	// hash for the storage key.
	if err := rxPacket.PHYPayload.SetMIC(lorawan.AES128Key{}); err != nil {
		return fmt.Errorf("set mic error: %s", err)
	}

	mic := hex.EncodeToString(rxPacket.PHYPayload.MIC[:])
	key := fmt.Sprintf(CollectKeyTempl, mic)
	lockKey := fmt.Sprintf(CollectLockKeyTempl, mic)

	// this way we can set a really low DeduplicationDelay for testing, without
	// the risk that the set already expired in redis on read
	deduplicationTTL := common.DeduplicationDelay * 2
	if deduplicationTTL < time.Millisecond*200 {
		deduplicationTTL = time.Millisecond * 200
	}

	c.Send("MULTI")
	c.Send("SADD", key, buf.Bytes())
	c.Send("PEXPIRE", key, int64(deduplicationTTL)/int64(time.Millisecond))
	_, err := c.Do("EXEC")
	if err != nil {
		return fmt.Errorf("add rx packet to collect set error: %s", err)
	}

	// acquire a lock on processing this packet
	_, err = redis.String((c.Do("SET", lockKey, "lock", "PX", int64(deduplicationTTL)/int64(time.Millisecond), "NX")))
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
	time.Sleep(common.DeduplicationDelay)

	// collect all packets from the set
	var rxPacketWithRXInfoSet models.RXPacket
	payloads, err := redis.ByteSlices(c.Do("SMEMBERS", key))
	if err != nil {
		return fmt.Errorf("get collect set members error: %s", err)
	}
	if len(payloads) == 0 {
		return errors.New("zero items in collect set")
	}

	for i, b := range payloads {
		var packet gw.RXPacket
		if err := gob.NewDecoder(bytes.NewReader(b)).Decode(&packet); err != nil {
			return fmt.Errorf("decode rx packet error: %s", err)
		}

		if i == 0 {
			rxPacketWithRXInfoSet.PHYPayload = packet.PHYPayload
		}
		rxPacketWithRXInfoSet.RXInfoSet = append(rxPacketWithRXInfoSet.RXInfoSet, packet.RXInfo)
	}

	sort.Sort(rxPacketWithRXInfoSet.RXInfoSet)
	return callback(rxPacketWithRXInfoSet)
}
