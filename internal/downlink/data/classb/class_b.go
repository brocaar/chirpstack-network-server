package classb

import (
	"crypto/aes"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/brocaar/lorawan"
	"github.com/pkg/errors"

	"github.com/brocaar/loraserver/internal/gps"
)

const (
	beaconPeriod   = 128 * time.Second
	beaconReserved = 2120 * time.Millisecond
	beaconGuard    = 3 * time.Second
	beaconWindow   = 122880 * time.Millisecond
	pingPeriodBase = 4096 // 2^12
)

// GetBeaconStartForTime returns the beacon start time as a duration
// since GPS epoch for the given time.Time.
func GetBeaconStartForTime(ts time.Time) time.Duration {
	gpsTime := gps.Time(ts).TimeSinceGPSEpoch()

	return gpsTime - (gpsTime % beaconPeriod)
}

// GetPingOffset returns the ping offset for the given beacon.
func GetPingOffset(beacon time.Duration, devAddr lorawan.DevAddr, pingNb int) (time.Duration, error) {
	if beacon%beaconPeriod != 0 {
		return 0, fmt.Errorf("beacon must be a multiple of %s", beaconPeriod)
	}

	devAddrBytes, err := devAddr.MarshalBinary()
	if err != nil {
		return 0, errors.Wrap(err, "marshal devaddr error")
	}

	pingPeriod := pingPeriodBase / pingNb
	beaconTime := uint32(int64(beacon/time.Second) % (2 << 31))

	key := lorawan.AES128Key{} // 16 x 0x00
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return 0, errors.Wrap(err, "new cipher error")
	}

	if block.BlockSize() != 16 {
		return 0, errors.New("block size of 16 was expected")
	}

	b := make([]byte, len(key))
	rand := make([]byte, len(key))

	binary.LittleEndian.PutUint32(b[0:4], beaconTime)
	copy(b[4:8], devAddrBytes)
	block.Encrypt(rand, b)

	return time.Duration((int(rand[0])+int(rand[1])*256)%pingPeriod) * time.Millisecond, nil
}
