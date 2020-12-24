package uplink

import (
	"encoding/hex"
	"fmt"
	"time"

	"github.com/gofrs/uuid"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/brocaar/chirpstack-network-server/internal/band"
	"github.com/brocaar/chirpstack-network-server/internal/helpers"
	"github.com/brocaar/chirpstack-network-server/internal/models"
	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/brocaar/lorawan"
)

// Templates used for generating Redis keys
const (
	CollectKeyTempl     = "lora:ns:rx:collect:%s:%s"
	CollectLockKeyTempl = "lora:ns:rx:collect:%s:%s:lock"
)

// collectAndCallOnce collects the package, sleeps the configured duration and
// calls the callback only once with a slice of packets, sorted by signal
// strength (strongest at index 0). This method exists since multiple gateways
// are able to receive the same packet, but the packet needs to processed
// only once.
// It is safe to collect the same packet received by the same gateway twice.
// Since the underlying storage type is a set, the result will always be a
// unique set per gateway MAC and packet MIC.
func collectAndCallOnce(rxPacket gw.UplinkFrame, callback func(packet models.RXPacket) error) error {
	phyKey := hex.EncodeToString(rxPacket.PhyPayload)
	txInfoB, err := proto.Marshal(rxPacket.TxInfo)
	if err != nil {
		return errors.Wrap(err, "marshal protobuf error")
	}
	txInfoHEX := hex.EncodeToString(txInfoB)

	key := fmt.Sprintf(CollectKeyTempl, txInfoHEX, phyKey)
	lockKey := fmt.Sprintf(CollectLockKeyTempl, txInfoHEX, phyKey)

	// this way we can set a really low DeduplicationDelay for testing, without
	// the risk that the set already expired in redis on read
	deduplicationTTL := deduplicationDelay * 2
	if deduplicationTTL < time.Millisecond*200 {
		deduplicationTTL = time.Millisecond * 200
	}

	if err := collectAndCallOncePut(key, deduplicationTTL, rxPacket); err != nil {
		return err
	}

	if locked, err := collectAndCallOnceLocked(lockKey, deduplicationTTL); err != nil || locked {
		// when locked == true, err == nil
		return err
	}

	// wait the configured amount of time, more packets might be received
	// from other gateways
	time.Sleep(deduplicationDelay)

	// collect all packets from the set
	payloads, err := collectAndCallOnceCollect(key)
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
		out.GatewayIsPrivate = make(map[lorawan.EUI64]bool)
		out.GatewayServiceProfile = make(map[lorawan.EUI64]uuid.UUID)
	}

	return callback(out)
}

func collectAndCallOncePut(key string, ttl time.Duration, rxPacket gw.UplinkFrame) error {
	b, err := proto.Marshal(&rxPacket)
	if err != nil {
		return errors.Wrap(err, "marshal uplink frame error")
	}

	pipe := storage.RedisClient().TxPipeline()
	pipe.SAdd(key, b)
	pipe.PExpire(key, ttl)

	_, err = pipe.Exec()
	if err != nil {
		return errors.Wrap(err, "add uplink frame to set error")
	}

	return nil
}

func collectAndCallOnceLocked(key string, ttl time.Duration) (bool, error) {
	// this way we can set a really low DeduplicationDelay for testing, without
	// the risk that the set already expired in redis on read
	deduplicationTTL := deduplicationDelay * 2
	if deduplicationTTL < time.Millisecond*200 {
		deduplicationTTL = time.Millisecond * 200
	}

	set, err := storage.RedisClient().SetNX(key, "lock", ttl).Result()
	if err != nil {
		return false, errors.Wrap(err, "acquire deduplication lock error")
	}

	// Set is true when we were able to set the lock, we return true if it
	// was already locked.
	return !set, nil
}

func collectAndCallOnceCollect(key string) ([][]byte, error) {
	pipe := storage.RedisClient().Pipeline()
	val := pipe.SMembers(key)
	pipe.Del(key)

	if _, err := pipe.Exec(); err != nil {
		return nil, errors.Wrap(err, "get set members error")
	}

	var out [][]byte
	vals := val.Val()

	for i := range vals {
		out = append(out, []byte(vals[i]))
	}

	return out, nil
}
