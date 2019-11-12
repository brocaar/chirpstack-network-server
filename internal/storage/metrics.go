package storage

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/brocaar/chirpstack-network-server/internal/logging"
	"github.com/gomodule/redigo/redis"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
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
	timeLocation         = time.Local
	aggregationIntervals []AggregationInterval
	metricsMinuteTTL     time.Duration
	metricsHourTTL       time.Duration
	metricsDayTTL        time.Duration
	metricsMonthTTL      time.Duration
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

// SetAggregationIntervals sets the metrics aggregation to the given intervals.
func SetAggregationIntervals(intervals []AggregationInterval) error {
	aggregationIntervals = intervals
	return nil
}

// SetMetricsTTL sets the storage TTL.
func SetMetricsTTL(minute, hour, day, month time.Duration) {
	metricsMinuteTTL = minute
	metricsHourTTL = hour
	metricsDayTTL = day
	metricsMonthTTL = month
}

// SaveMetrics stores the given metrics into Redis.
func SaveMetrics(ctx context.Context, p *redis.Pool, name string, metrics MetricsRecord) error {
	for _, agg := range aggregationIntervals {
		if err := SaveMetricsForInterval(ctx, p, agg, name, metrics); err != nil {
			return errors.Wrap(err, "save metrics for interval error")
		}
	}

	log.WithFields(log.Fields{
		"name":        name,
		"aggregation": aggregationIntervals,
		"ctx_id":      ctx.Value(logging.ContextIDKey),
	}).Info("metrics saved")

	return nil
}

// SaveMetricsForInterval aggregates and stores the given metrics.
func SaveMetricsForInterval(ctx context.Context, p *redis.Pool, agg AggregationInterval, name string, metrics MetricsRecord) error {
	if len(metrics.Metrics) == 0 {
		return nil
	}

	c := p.Get()
	defer c.Close()
	var exp int64

	// handle aggregation
	ts := metrics.Time.In(timeLocation)
	switch agg {
	case AggregationMinute:
		// truncate timestamp to minute precision
		ts = time.Date(ts.Year(), ts.Month(), ts.Day(), ts.Hour(), ts.Minute(), 0, 0, timeLocation)
		exp = int64(metricsMinuteTTL) / int64(time.Millisecond)
	case AggregationHour:
		// truncate timestamp to hour precision
		ts = time.Date(ts.Year(), ts.Month(), ts.Day(), ts.Hour(), 0, 0, 0, timeLocation)
		exp = int64(metricsHourTTL) / int64(time.Millisecond)
	case AggregationDay:
		// truncate timestamp to day precision
		ts = time.Date(ts.Year(), ts.Month(), ts.Day(), 0, 0, 0, 0, timeLocation)
		exp = int64(metricsDayTTL) / int64(time.Millisecond)
	case AggregationMonth:
		// truncate timestamp to month precision
		ts = time.Date(ts.Year(), ts.Month(), 1, 0, 0, 0, 0, timeLocation)
		exp = int64(metricsMonthTTL) / int64(time.Millisecond)
	default:
		return fmt.Errorf("unexepcted aggregation interval: %s", agg)
	}

	key := fmt.Sprintf(metricsKeyTempl, name, agg, ts.Unix())

	c.Send("MULTI")
	for k, v := range metrics.Metrics {
		c.Send("HINCRBYFLOAT", key, k, v)
	}
	c.Send("PEXPIRE", key, exp)

	if _, err := c.Do("EXEC"); err != nil {
		return errors.Wrap(err, "exec error")
	}

	log.WithFields(log.Fields{
		"name":        name,
		"aggregation": agg,
		"ctx_id":      ctx.Value(logging.ContextIDKey),
	}).Debug("metrics saved")

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
