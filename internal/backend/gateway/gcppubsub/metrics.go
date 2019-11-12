package gcppubsub

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	ec = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "backend_gcp_pub_sub_event_count",
		Help: "The number of received events by the GCP Pub/Sub backend (per event type).",
	}, []string{"event"})

	cc = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "backend_gcp_pub_sub_command_count",
		Help: "The number of published commands by the GCP Pub/Sub backend (per command type).",
	}, []string{"command"})
)

func gcpEventCounter(e string) prometheus.Counter {
	return ec.With(prometheus.Labels{"event": e})
}

func gcpCommandCounter(c string) prometheus.Counter {
	return cc.With(prometheus.Labels{"command": c})
}
