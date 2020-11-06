package uplink

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	uc = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "uplink_counter",
		Help: "The number of handled uplink frames by the Server (per message type).",
	}, []string{"mType"})
	ufce = promauto.NewCounter(prometheus.CounterOpts{
		Name: "uplink_frame_error_count",
		Help: "The number of uplink frames that failed to be processed .",
	})
)

func uplinkFrameCounter(e string) prometheus.Counter {
	return uc.With(prometheus.Labels{"mType": e})
}

func uplinkFrameErrorCount() prometheus.Counter {
	return ufce
}
