package uplink

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"sort"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/pkg/errors"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/models"
)

// Templates used for generating Redis keys
const (
	CollectKeyTempl     = "lora:ns:rx:collect:%s"
	CollectLockKeyTempl = "lora:ns:rx:collect:%s:lock"
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
	// The text representation of the PHYPayload is used as key.
	phyB, err := rxPacket.PHYPayload.MarshalText()
	if err != nil {
		return errors.Wrap(err, "marshal to text error")
	}

	key := fmt.Sprintf(CollectKeyTempl, string(phyB))
	lockKey := fmt.Sprintf(CollectLockKeyTempl, string(phyB))

	// this way we can set a really low DeduplicationDelay for testing, without
	// the risk that the set already expired in redis on read
	deduplicationTTL := common.DeduplicationDelay * 2
	if deduplicationTTL < time.Millisecond*200 {
		deduplicationTTL = time.Millisecond * 200
	}

	c.Send("MULTI")
	c.Send("SADD", key, buf.Bytes())
	c.Send("PEXPIRE", key, int64(deduplicationTTL)/int64(time.Millisecond))
	_, err = c.Do("EXEC")
	if err != nil {
		return fmt.Errorf("add rx packet to collect set error: %s", err)
	}

	// acquire a lock on processing this packet
	_, err = redis.String(c.Do("SET", lockKey, "lock", "PX", int64(deduplicationTTL)/int64(time.Millisecond), "NX"))
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
	payloads, err := redis.ByteSlices(c.Do("SMEMBERS", key))
	if err != nil {
		return fmt.Errorf("get collect set members error: %s", err)
	}
	if len(payloads) == 0 {
		return errors.New("zero items in collect set")
	}

	var out models.RXPacket
	for _, b := range payloads {
		var packet gw.RXPacket
		if err := gob.NewDecoder(bytes.NewReader(b)).Decode(&packet); err != nil {
			return errors.Wrap(err, "decode rx packet error")
		}

		out.PHYPayload = packet.PHYPayload
		out.TXInfo = models.TXInfo{
			Frequency: packet.RXInfo.Frequency,
			DataRate:  packet.RXInfo.DataRate,
			CodeRate:  packet.RXInfo.CodeRate,
		}

		out.RXInfoSet = append(out.RXInfoSet, models.RXInfo{
			MAC:               packet.RXInfo.MAC,
			Time:              packet.RXInfo.Time,
			TimeSinceGPSEpoch: packet.RXInfo.TimeSinceGPSEpoch,
			Timestamp:         packet.RXInfo.Timestamp,
			RSSI:              packet.RXInfo.RSSI,
			LoRaSNR:           packet.RXInfo.LoRaSNR,
			Board:             packet.RXInfo.Board,
			Antenna:           packet.RXInfo.Antenna,
		})
	}

	sort.Sort(out.RXInfoSet)
	return callback(out)
}
