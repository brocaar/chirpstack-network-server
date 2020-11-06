package stats

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	gatewayID = "gateway_id"
)

var (
	ug = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "stats_unknown_gateway_count",
		Help: "The number of uplinks from unknown gateways (per gateway).",
	}, []string{gatewayID})
)

func statsUnknownGateway(g string) prometheus.Counter {
	return ug.With(prometheus.Labels{gatewayID: g})
}
