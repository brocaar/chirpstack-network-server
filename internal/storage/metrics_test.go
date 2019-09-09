package storage

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/brocaar/loraserver/internal/test"
)

func (ts *StorageTestSuite) TestMetrics() {
	assert := require.New(ts.T())
	loc, err := time.LoadLocation("Europe/Amsterdam")
	assert.NoError(err)

	SetMetricsTTL(time.Minute, time.Minute, time.Minute, time.Minute)

	tests := []struct {
		Name         string
		MetricsName  string
		LocationName string
		Interval     AggregationInterval
		SaveMetrics  []MetricsRecord
		GetStart     time.Time
		GetEnd       time.Time
		GetMetrics   []MetricsRecord
	}{
		{
			Name:         "minute aggregation",
			LocationName: "Europe/Amsterdam",
			Interval:     AggregationMinute,
			SaveMetrics: []MetricsRecord{
				{
					Time: time.Date(2018, 1, 1, 1, 1, 1, 0, loc),
					Metrics: map[string]float64{
						"foo": 1,
						"bar": 2,
					},
				},
				{
					Time: time.Date(2018, 1, 1, 1, 1, 2, 0, loc),
					Metrics: map[string]float64{
						"foo": 3,
						"bar": 4,
					},
				},
				{
					Time: time.Date(2018, 1, 1, 1, 2, 0, 0, loc),
					Metrics: map[string]float64{
						"foo": 5,
						"bar": 6,
					},
				},
			},
			GetMetrics: []MetricsRecord{
				{
					Time: time.Date(2018, 1, 1, 1, 1, 0, 0, loc),
					Metrics: map[string]float64{
						"foo": 4,
						"bar": 6,
					},
				},
				{
					Time: time.Date(2018, 1, 1, 1, 2, 0, 0, loc),
					Metrics: map[string]float64{
						"foo": 5,
						"bar": 6,
					},
				},
			},
			GetStart: time.Date(2018, 1, 1, 1, 1, 1, 0, loc),
			GetEnd:   time.Date(2018, 1, 1, 1, 2, 1, 0, loc),
		},
		{
			Name:         "hour aggregation",
			LocationName: "Europe/Amsterdam",
			Interval:     AggregationHour,
			SaveMetrics: []MetricsRecord{
				{
					Time: time.Date(2018, 1, 1, 1, 1, 0, 0, loc),
					Metrics: map[string]float64{
						"foo": 1,
						"bar": 2,
					},
				},
				{
					Time: time.Date(2018, 1, 1, 1, 2, 0, 0, loc),
					Metrics: map[string]float64{
						"foo": 3,
						"bar": 4,
					},
				},
				{
					Time: time.Date(2018, 1, 1, 2, 1, 0, 0, loc),
					Metrics: map[string]float64{
						"foo": 5,
						"bar": 6,
					},
				},
			},
			GetMetrics: []MetricsRecord{
				{
					Time: time.Date(2018, 1, 1, 1, 0, 0, 0, loc),
					Metrics: map[string]float64{
						"foo": 4,
						"bar": 6,
					},
				},
				{
					Time: time.Date(2018, 1, 1, 2, 0, 0, 0, loc),
					Metrics: map[string]float64{
						"foo": 5,
						"bar": 6,
					},
				},
			},
			GetStart: time.Date(2018, 1, 1, 1, 0, 0, 0, loc),
			GetEnd:   time.Date(2018, 1, 1, 2, 0, 0, 0, loc),
		},
		{
			Name:         "day aggregation",
			LocationName: "Europe/Amsterdam",
			Interval:     AggregationDay,
			SaveMetrics: []MetricsRecord{
				{
					Time: time.Date(2018, 1, 1, 1, 0, 0, 0, loc),
					Metrics: map[string]float64{
						"foo": 1,
						"bar": 2,
					},
				},
				{
					Time: time.Date(2018, 1, 1, 2, 0, 0, 0, loc),
					Metrics: map[string]float64{
						"foo": 3,
						"bar": 4,
					},
				},
				{
					Time: time.Date(2018, 1, 2, 1, 0, 0, 0, loc),
					Metrics: map[string]float64{
						"foo": 5,
						"bar": 6,
					},
				},
			},
			GetMetrics: []MetricsRecord{
				{
					Time: time.Date(2018, 1, 1, 0, 0, 0, 0, loc),
					Metrics: map[string]float64{
						"foo": 4,
						"bar": 6,
					},
				},
				{
					Time: time.Date(2018, 1, 2, 0, 0, 0, 0, loc),
					Metrics: map[string]float64{
						"foo": 5,
						"bar": 6,
					},
				},
			},
			GetStart: time.Date(2018, 1, 1, 1, 0, 0, 0, loc),
			GetEnd:   time.Date(2018, 1, 2, 1, 0, 0, 0, loc),
		},
		{
			Name:         "month aggregation",
			LocationName: "Europe/Amsterdam",
			Interval:     AggregationMonth,
			SaveMetrics: []MetricsRecord{
				{
					Time: time.Date(2018, 1, 1, 0, 0, 0, 0, loc),
					Metrics: map[string]float64{
						"foo": 1,
						"bar": 2,
					},
				},
				{
					Time: time.Date(2018, 1, 2, 0, 0, 0, 0, loc),
					Metrics: map[string]float64{
						"foo": 3,
						"bar": 4,
					},
				},
				{
					Time: time.Date(2018, 2, 1, 0, 0, 0, 0, loc),
					Metrics: map[string]float64{
						"foo": 5,
						"bar": 6,
					},
				},
			},
			GetMetrics: []MetricsRecord{
				{
					Time: time.Date(2018, 1, 1, 0, 0, 0, 0, loc),
					Metrics: map[string]float64{
						"foo": 4,
						"bar": 6,
					},
				},
				{
					Time: time.Date(2018, 2, 1, 0, 0, 0, 0, loc),
					Metrics: map[string]float64{
						"foo": 5,
						"bar": 6,
					},
				},
			},
			GetStart: time.Date(2018, 1, 1, 0, 0, 0, 0, loc),
			GetEnd:   time.Date(2018, 2, 1, 0, 0, 0, 0, loc),
		},
		{
			Name:         "earlier start date",
			LocationName: "Europe/Amsterdam",
			Interval:     AggregationDay,
			SaveMetrics: []MetricsRecord{
				{
					Time: time.Date(2018, 1, 1, 1, 0, 0, 0, loc),
					Metrics: map[string]float64{
						"foo": 1,
						"bar": 2,
					},
				},
				{
					Time: time.Date(2018, 1, 1, 2, 0, 0, 0, loc),
					Metrics: map[string]float64{
						"foo": 3,
						"bar": 4,
					},
				},
				{
					Time: time.Date(2018, 1, 2, 1, 0, 0, 0, loc),
					Metrics: map[string]float64{
						"foo": 5,
						"bar": 6,
					},
				},
			},
			GetMetrics: []MetricsRecord{
				{
					Time:    time.Date(2017, 12, 31, 0, 0, 0, 0, loc),
					Metrics: map[string]float64{},
				},
				{
					Time: time.Date(2018, 1, 1, 0, 0, 0, 0, loc),
					Metrics: map[string]float64{
						"foo": 4,
						"bar": 6,
					},
				},
				{
					Time: time.Date(2018, 1, 2, 0, 0, 0, 0, loc),
					Metrics: map[string]float64{
						"foo": 5,
						"bar": 6,
					},
				},
			},
			GetStart: time.Date(2017, 12, 31, 0, 0, 0, 0, loc),
			GetEnd:   time.Date(2018, 1, 2, 1, 0, 0, 0, loc),
		},
		{
			Name:         "exact start date",
			LocationName: "Europe/Amsterdam",
			Interval:     AggregationDay,
			SaveMetrics: []MetricsRecord{
				{
					Time: time.Date(2018, 1, 1, 1, 0, 0, 0, loc),
					Metrics: map[string]float64{
						"foo": 1,
						"bar": 2,
					},
				},
				{
					Time: time.Date(2018, 1, 1, 2, 0, 0, 0, loc),
					Metrics: map[string]float64{
						"foo": 3,
						"bar": 4,
					},
				},
				{
					Time: time.Date(2018, 1, 2, 1, 0, 0, 0, loc),
					Metrics: map[string]float64{
						"foo": 5,
						"bar": 6,
					},
				},
			},
			GetMetrics: []MetricsRecord{
				{
					Time: time.Date(2018, 1, 1, 0, 0, 0, 0, loc),
					Metrics: map[string]float64{
						"foo": 4,
						"bar": 6,
					},
				},
				{
					Time: time.Date(2018, 1, 2, 0, 0, 0, 0, loc),
					Metrics: map[string]float64{
						"foo": 5,
						"bar": 6,
					},
				},
			},
			GetStart: time.Date(2018, 1, 1, 0, 0, 0, 0, loc),
			GetEnd:   time.Date(2018, 1, 2, 1, 0, 0, 0, loc),
		},
		{
			Name:         "later end date",
			LocationName: "Europe/Amsterdam",
			Interval:     AggregationDay,
			SaveMetrics: []MetricsRecord{
				{
					Time: time.Date(2018, 1, 1, 1, 0, 0, 0, loc),
					Metrics: map[string]float64{
						"foo": 1,
						"bar": 2,
					},
				},
				{
					Time: time.Date(2018, 1, 1, 2, 0, 0, 0, loc),
					Metrics: map[string]float64{
						"foo": 3,
						"bar": 4,
					},
				},
				{
					Time: time.Date(2018, 1, 2, 1, 0, 0, 0, loc),
					Metrics: map[string]float64{
						"foo": 5,
						"bar": 6,
					},
				},
			},
			GetMetrics: []MetricsRecord{
				{
					Time: time.Date(2018, 1, 1, 0, 0, 0, 0, loc),
					Metrics: map[string]float64{
						"foo": 4,
						"bar": 6,
					},
				},
				{
					Time: time.Date(2018, 1, 2, 0, 0, 0, 0, loc),
					Metrics: map[string]float64{
						"foo": 5,
						"bar": 6,
					},
				},
				{
					Time:    time.Date(2018, 1, 3, 0, 0, 0, 0, loc),
					Metrics: map[string]float64{},
				},
			},
			GetStart: time.Date(2018, 1, 1, 1, 0, 0, 0, loc),
			GetEnd:   time.Date(2018, 1, 3, 1, 0, 0, 0, loc),
		},
	}

	for _, tst := range tests {
		ts.T().Run(tst.Name, func(t *testing.T) {
			assert := require.New(t)
			assert.NoError(SetTimeLocation(tst.LocationName))

			test.MustFlushRedis(ts.RedisPool())

			for _, metrics := range tst.SaveMetrics {
				assert.NoError(SaveMetricsForInterval(context.Background(), ts.RedisPool(), tst.Interval, "metrics_test", metrics))
			}

			metrics, err := GetMetrics(context.Background(), ts.RedisPool(), tst.Interval, "metrics_test", tst.GetStart, tst.GetEnd)
			assert.NoError(err)
			assert.EqualValues(tst.GetMetrics, metrics)
		})
	}
}
