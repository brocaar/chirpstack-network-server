package storage

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/pkg/errors"
)

// AggregationInterval defines the aggregation type.
type AggregationInterval string

// Metrics aggregation intervals.
const (
	AggregationMinute AggregationInterval = "MINUTE"
	AggregationHour   AggregationInterval = "HOUR"
	AggregationDay    AggregationInterval = "DAY"
	AggregationMonth  AggregationInterval = "MONTH"
)

const (
	metricsKeyTempl = "lora:ns:metrics:%s:%s:%d" // metrics key (identifier | aggregation | timestamp)

)

var (
	timeLocation = time.Local
)

// MetricsRecord holds a single metrics record.
type MetricsRecord struct {
	Time    time.Time
	Metrics map[string]float64
}

// SetTimeLocation sets the time location.
func SetTimeLocation(name string) error {
	var err error
	timeLocation, err = time.LoadLocation(name)
	if err != nil {
		return errors.Wrap(err, "load location error")
	}
	return nil
}

// GetMetrics returns the metrics for the requested aggregation interval.
func GetMetrics(ctx context.Context, p *redis.Pool, agg AggregationInterval, name string, start, end time.Time) ([]MetricsRecord, error) {
	c := p.Get()
	defer c.Close()

	var keys []string
	var timestamps []time.Time

	start = start.In(timeLocation)
	end = end.In(timeLocation)

	// handle aggregation keys
	switch agg {
	case AggregationMinute:
		end = time.Date(end.Year(), end.Month(), end.Day(), end.Hour(), end.Minute(), 0, 0, timeLocation)
		for i := 0; ; i++ {
			ts := time.Date(start.Year(), start.Month(), start.Day(), start.Hour(), start.Minute()+i, 0, 0, timeLocation)
			if ts.After(end) {
				break
			}
			timestamps = append(timestamps, ts)
			keys = append(keys, fmt.Sprintf(metricsKeyTempl, name, agg, ts.Unix()))
		}
	case AggregationHour:
		end = time.Date(end.Year(), end.Month(), end.Day(), end.Hour(), 0, 0, 0, timeLocation)
		for i := 0; ; i++ {
			ts := time.Date(start.Year(), start.Month(), start.Day(), start.Hour()+i, 0, 0, 0, timeLocation)
			if ts.After(end) {
				break
			}
			timestamps = append(timestamps, ts)
			keys = append(keys, fmt.Sprintf(metricsKeyTempl, name, agg, ts.Unix()))
		}
	case AggregationDay:
		end = time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, timeLocation)
		for i := 0; ; i++ {
			ts := time.Date(start.Year(), start.Month(), start.Day()+i, 0, 0, 0, 0, timeLocation)
			if ts.After(end) {
				break
			}
			timestamps = append(timestamps, ts)
			keys = append(keys, fmt.Sprintf(metricsKeyTempl, name, agg, ts.Unix()))
		}
	case AggregationMonth:
		end = time.Date(end.Year(), end.Month(), 1, 0, 0, 0, 0, timeLocation)
		for i := 0; ; i++ {
			ts := time.Date(start.Year(), start.Month()+time.Month(i), 1, 0, 0, 0, 0, timeLocation)
			if ts.After(end) {
				break
			}
			timestamps = append(timestamps, ts)
			keys = append(keys, fmt.Sprintf(metricsKeyTempl, name, agg, ts.Unix()))
		}
	default:
		return nil, fmt.Errorf("unexepcted aggregation interval: %s", agg)
	}

	if len(keys) == 0 {
		return nil, nil
	}

	for _, k := range keys {
		c.Send("HGETALL", k)
	}
	c.Flush()

	var out []MetricsRecord

	for _, ts := range timestamps {
		metrics := MetricsRecord{
			Time:    ts,
			Metrics: make(map[string]float64),
		}

		vals, err := redis.StringMap(c.Receive())
		if err != nil {
			return nil, errors.Wrap(err, "receive stringmap error")
		}

		for k, v := range vals {
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return nil, errors.Wrap(err, "parse float error")
			}

			metrics.Metrics[k] = f
		}

		out = append(out, metrics)
	}

	return out, nil
}
