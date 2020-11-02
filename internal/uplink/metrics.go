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
)

func uplinkFrameCounter(e string) prometheus.Counter {
	return uc.With(prometheus.Labels{"mType": e})
}
