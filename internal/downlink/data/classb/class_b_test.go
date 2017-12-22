package classb

import (
	"testing"
	"time"

	"github.com/brocaar/lorawan"

	"github.com/brocaar/loraserver/internal/gps"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGetCurrentBeaconStart(t *testing.T) {
	Convey("When calling GetCurrentBeaconStart", t, func() {
		d := GetBeaconStartForTime(time.Now())

		Convey("Then the returned value > 0", func() {
			So(d, ShouldBeGreaterThan, 0)
		})

		Convey("Then the returned value is a multiple of 128 seconds", func() {
			So(d%beaconPeriod, ShouldEqual, 0)
		})

		Convey("Then the returned value is less than 128 seconds ago", func() {
			ts := time.Time(gps.NewFromTimeSinceGPSEpoch(d))
			So(ts.Before(time.Now()), ShouldBeTrue)

			So(time.Now().Sub(ts), ShouldBeLessThan, beaconPeriod)
		})
	})
}

func TestGetPingOffset(t *testing.T) {
	for k := uint(0); k < 8; k++ {
		var beacon time.Duration
		pingNb := 2 << (k - 1)
		if pingNb == 0 {
			pingNb = 1
		}
		pingPeriod := pingPeriodBase / pingNb

		for test := 0; test < 100000; test++ {
			offset, err := GetPingOffset(beacon, lorawan.DevAddr{}, pingNb)
			if err != nil {
				t.Fatal(err)
			}

			if offset > time.Duration(pingPeriod-1)*time.Millisecond {
				t.Errorf("unexpected offset %d at pingNb %d test %d", offset, pingNb, test)
			}

			beacon += beaconPeriod
		}
	}
}
