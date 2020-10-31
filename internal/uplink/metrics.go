package uplink

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	uc = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "uplink_histogram",
		Help: "The distribution of handled uplink frames by the Server (per message type).",
	}, []string{"mType"})
)

func uplinkFrameObserver(e string) prometheus.Observer {
	return uc.With(prometheus.Labels{"mType": e})
}
