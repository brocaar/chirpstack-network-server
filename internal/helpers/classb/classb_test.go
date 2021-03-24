package classb

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/brocaar/chirpstack-network-server/internal/gps"
	"github.com/brocaar/lorawan"
)

func TestGetBeaconStartForTime(t *testing.T) {
	gpsEpochTime := time.Date(1980, time.January, 6, 0, 0, 0, 0, time.UTC)

	t.Run("For GPS epoch time", func(t *testing.T) {
		assert := require.New(t)
		d := GetBeaconStartForTime(gpsEpochTime)
		assert.EqualValues(0, d)
	})

	t.Run("For time.Now", func(t *testing.T) {
		assert := require.New(t)

		// Greater than 0
		d := GetBeaconStartForTime(time.Now())
		assert.Greater(int64(d), int64(0))

		// Multiple of 128 seconds
		assert.EqualValues(int64(0), int64(d%beaconPeriod))

		// Less than 128 seconds ago.
		ts := time.Time(gps.NewFromTimeSinceGPSEpoch(d))
		assert.True(ts.Before(time.Now()))
		assert.Less(int64(time.Now().Sub(ts)), int64(beaconPeriod))
	})
}

func TestGetPingOffset(t *testing.T) {
	for k := uint(0); k < 8; k++ {
		var beacon time.Duration
		pingNb := 1 << k
		pingPeriod := pingPeriodBase / pingNb

		for test := 0; test < 100000; test++ {
			offset, err := GetPingOffset(beacon, lorawan.DevAddr{}, pingNb)
			if err != nil {
				t.Fatal(err)
			}

			if offset > pingPeriod-1 {
				t.Errorf("unexpected offset %d at pingNb %d test %d", offset, pingNb, test)
			}

			beacon += beaconPeriod
		}
	}
}

func TestGetNextPingSlotAfter(t *testing.T) {
	tests := []struct {
		After                    time.Duration
		DevAddr                  lorawan.DevAddr
		PingNb                   int
		ExpectedGPSEpochDuration string
		ExpectedError            error
	}{
		{
			After:                    0,
			DevAddr:                  lorawan.DevAddr{},
			PingNb:                   1,
			ExpectedGPSEpochDuration: "1m14.3s",
		},
		{
			After:                    2 * time.Minute,
			DevAddr:                  lorawan.DevAddr{},
			PingNb:                   1,
			ExpectedGPSEpochDuration: "3m5.62s",
		},
		{
			After:                    0,
			DevAddr:                  lorawan.DevAddr{},
			PingNb:                   2,
			ExpectedGPSEpochDuration: "12.86s",
		},
		{
			After:                    13 * time.Second,
			DevAddr:                  lorawan.DevAddr{},
			PingNb:                   2,
			ExpectedGPSEpochDuration: "1m14.3s",
		},
		{
			After:                    124 * time.Second,
			DevAddr:                  lorawan.DevAddr{},
			PingNb:                   128,
			ExpectedGPSEpochDuration: "2m4.22s",
		},
	}

	for _, test := range tests {
		var expDuration time.Duration

		if test.ExpectedGPSEpochDuration != "" {
			var err error
			expDuration, err = time.ParseDuration(test.ExpectedGPSEpochDuration)
			if err != nil {
				t.Fatal(err)
			}
		}

		gpsEpochDuration, err := GetNextPingSlotAfter(test.After, test.DevAddr, test.PingNb)
		if err != test.ExpectedError {
			t.Errorf("expected error %s, got: %s", test.ExpectedError, err)
		}

		if gpsEpochDuration != expDuration {
			t.Errorf("expected gps epoch duration %s, got %s", test.ExpectedGPSEpochDuration, gpsEpochDuration)
		}
	}
}
