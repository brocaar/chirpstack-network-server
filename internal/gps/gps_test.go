package gps

import (
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTime(t *testing.T) {
	Convey("Given a set of tests", t, func() {
		tests := []struct {
			Time              time.Time
			TimeSinceGPSEpoch time.Duration
		}{
			{Time: gpsEpochTime, TimeSinceGPSEpoch: 0},
			{Time: time.Date(2010, time.January, 28, 16, 36, 24, 0, time.UTC), TimeSinceGPSEpoch: 948731799 * time.Second},
			{Time: time.Date(2025, time.July, 14, 0, 0, 0, 0, time.UTC), TimeSinceGPSEpoch: 1436486418 * time.Second},
			{Time: time.Date(2012, time.June, 30, 23, 59, 59, 0, time.UTC), TimeSinceGPSEpoch: 1025136014 * time.Second},
			{Time: time.Date(2012, time.July, 1, 0, 0, 0, 0, time.UTC), TimeSinceGPSEpoch: 1025136016 * time.Second},
		}

		for i, test := range tests {
			Convey(fmt.Sprintf("Testing: %s == %s [%d]", test.Time, test.TimeSinceGPSEpoch, i), func() {
				gpsTime := Time(test.Time)
				So(gpsTime.TimeSinceGPSEpoch(), ShouldEqual, test.TimeSinceGPSEpoch)

				gpsTime = NewFromTimeSinceGPSEpoch(test.TimeSinceGPSEpoch)
				So(time.Time(gpsTime).Equal(test.Time), ShouldBeTrue)
			})
		}
	})
}
