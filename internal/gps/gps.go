package gps

import (
	"fmt"
	"time"
)

var gpsEpochTime = time.Date(1980, time.January, 6, 0, 0, 0, 0, time.UTC)

var leapSecondsTable = []struct {
	Time     time.Time
	Duration time.Duration
}{
	{Time: time.Date(1981, time.June, 30, 23, 59, 59, 0, time.UTC), Duration: time.Second},
	{Time: time.Date(1982, time.June, 30, 23, 59, 59, 0, time.UTC), Duration: time.Second},
	{Time: time.Date(1983, time.June, 30, 23, 59, 59, 0, time.UTC), Duration: time.Second},
	{Time: time.Date(1985, time.June, 30, 23, 59, 59, 0, time.UTC), Duration: time.Second},
	{Time: time.Date(1987, time.December, 31, 23, 59, 59, 0, time.UTC), Duration: time.Second},
	{Time: time.Date(1989, time.December, 31, 23, 59, 59, 0, time.UTC), Duration: time.Second},
	{Time: time.Date(1990, time.December, 31, 23, 59, 59, 0, time.UTC), Duration: time.Second},
	{Time: time.Date(1992, time.June, 30, 23, 59, 59, 0, time.UTC), Duration: time.Second},
	{Time: time.Date(1993, time.June, 30, 23, 59, 59, 0, time.UTC), Duration: time.Second},
	{Time: time.Date(1994, time.June, 30, 23, 59, 59, 0, time.UTC), Duration: time.Second},
	{Time: time.Date(1995, time.December, 31, 23, 59, 59, 0, time.UTC), Duration: time.Second},
	{Time: time.Date(1997, time.June, 30, 23, 59, 59, 0, time.UTC), Duration: time.Second},
	{Time: time.Date(1998, time.December, 31, 23, 59, 59, 0, time.UTC), Duration: time.Second},
	{Time: time.Date(2005, time.December, 31, 23, 59, 59, 0, time.UTC), Duration: time.Second},
	{Time: time.Date(2008, time.December, 31, 23, 59, 59, 0, time.UTC), Duration: time.Second},
	{Time: time.Date(2012, time.June, 30, 23, 59, 59, 0, time.UTC), Duration: time.Second},
	{Time: time.Date(2015, time.June, 30, 23, 59, 59, 0, time.UTC), Duration: time.Second},
	{Time: time.Date(2016, time.December, 31, 23, 59, 59, 0, time.UTC), Duration: time.Second},
}

// Time represents a GPS time wrapper.
type Time time.Time

// NewFromTimeSinceGPSEpoch returns a new Time given a time since GPS epoch
// and will apply the leap second correction.
func NewFromTimeSinceGPSEpoch(sinceEpoch time.Duration) Time {
	t := gpsEpochTime.Add(sinceEpoch)
	for _, ls := range leapSecondsTable {
		if ls.Time.Before(t) {
			t = t.Add(-ls.Duration)
		}
	}

	return Time(t)
}

// TimeSinceGPSEpoch returns the time duration since GPS epoch, corrected with
// the leap seconds.
func (t Time) TimeSinceGPSEpoch() time.Duration {
	var offset time.Duration
	for _, ls := range leapSecondsTable {
		if ls.Time.Before(time.Time(t)) {
			offset += ls.Duration
		}
	}

	return time.Time(t).Sub(gpsEpochTime) + offset
}

// String implements the Stringer interface.
func (t Time) String() string {
	return fmt.Sprintf("%s (%s since GPS epoch)", time.Time(t).String(), t.TimeSinceGPSEpoch().String())
}
