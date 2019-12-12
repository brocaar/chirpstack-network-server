package uplink

import (
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/gomodule/redigo/redis"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/brocaar/chirpstack-network-server/internal/band"
	"github.com/brocaar/chirpstack-network-server/internal/helpers"
	"github.com/brocaar/chirpstack-network-server/internal/models"
	"github.com/brocaar/lorawan"
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
func collectAndCallOnce(p *redis.Pool, rxPacket gw.UplinkFrame, callback func(packet models.RXPacket) error) error {
	phyKey := hex.EncodeToString(rxPacket.PhyPayload)
	key := fmt.Sprintf(CollectKeyTempl, phyKey)
	lockKey := fmt.Sprintf(CollectLockKeyTempl, phyKey)

	// this way we can set a really low DeduplicationDelay for testing, without
	// the risk that the set already expired in redis on read
	deduplicationTTL := deduplicationDelay * 2
	if deduplicationTTL < time.Millisecond*200 {
		deduplicationTTL = time.Millisecond * 200
	}

	if err := collectAndCallOncePut(p, key, deduplicationTTL, rxPacket); err != nil {
		return err
	}

	if locked, err := collectAndCallOnceLocked(p, lockKey, deduplicationTTL); err != nil || locked {
		// when locked == true, err == nil
		return err
	}

	// wait the configured amount of time, more packets might be received
	// from other gateways
	time.Sleep(deduplicationDelay)

	// collect all packets from the set
	payloads, err := collectAndCallOnceCollect(p, key)
	if err != nil {
		return errors.Wrap(err, "get deduplication set members error")
	}
	if len(payloads) == 0 {
		return errors.New("zero items in collect set")
	}

	var out models.RXPacket
	for i, b := range payloads {
		var uplinkFrame gw.UplinkFrame
		if err := proto.Unmarshal(b, &uplinkFrame); err != nil {
			return errors.Wrap(err, "unmarshal uplink frame error")
		}

		if uplinkFrame.TxInfo == nil {
			log.Warning("tx-info of uplink frame is empty, skipping")
			continue
		}

		if uplinkFrame.RxInfo == nil {
			log.Warning("rx-info of uplink frame is empty, skipping")
			continue
		}

		if i == 0 {
			var phy lorawan.PHYPayload
			if err := phy.UnmarshalBinary(uplinkFrame.PhyPayload); err != nil {
				return errors.Wrap(err, "unmarshal phypayload error")
			}

			out.PHYPayload = phy

			dr, err := helpers.GetDataRateIndex(true, uplinkFrame.TxInfo, band.Band())
			if err != nil {
				return errors.Wrap(err, "get data-rate index error")
			}
			out.DR = dr
		}

		out.TXInfo = uplinkFrame.TxInfo
		out.RXInfoSet = append(out.RXInfoSet, uplinkFrame.RxInfo)
	}

	sort.Sort(models.BySignalStrength(out.RXInfoSet))
	return callback(out)
}

func collectAndCallOncePut(p *redis.Pool, key string, ttl time.Duration, rxPacket gw.UplinkFrame) error {
	b, err := proto.Marshal(&rxPacket)
	if err != nil {
		return errors.Wrap(err, "marshal uplink frame error")
	}

	c := p.Get()
	defer c.Close()

	c.Send("MULTI")
	c.Send("SADD", key, b)
	c.Send("PEXPIRE", key, int64(ttl)/int64(time.Millisecond))
	_, err = c.Do("EXEC")
	if err != nil {
		return errors.Wrap(err, "add uplink frame to set error")
	}

	return nil
}

func collectAndCallOnceLocked(p *redis.Pool, key string, ttl time.Duration) (bool, error) {
	c := p.Get()
	defer c.Close()

	// this way we can set a really low DeduplicationDelay for testing, without
	// the risk that the set already expired in redis on read
	deduplicationTTL := deduplicationDelay * 2
	if deduplicationTTL < time.Millisecond*200 {
		deduplicationTTL = time.Millisecond * 200
	}

	_, err := redis.String(c.Do("SET", key, "lock", "PX", int64(ttl)/int64(time.Millisecond), "NX"))
	if err != nil {
		if err == redis.ErrNil {
			// the packet processing is already locked by an other process
			// so there is nothing to do anymore :-)
			return true, nil
		}
		return false, errors.Wrap(err, "acquire deduplication lock error")
	}

	return false, nil
}

func collectAndCallOnceCollect(p *redis.Pool, key string) ([][]byte, error) {
	c := p.Get()
	defer c.Close()

	return redis.ByteSlices(c.Do("SMEMBERS", key))
}
