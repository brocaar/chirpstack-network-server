package gateway

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	gatewayID = "gateway_id"
)

var (
	ug = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "uplink_unknown_gateway_count",
		Help: "The number of uplinks from unknown gateways (per gateway).",
	}, []string{gatewayID})
	gg = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "get_gateway_failed_count",
		Help: "The number of gateway lookups that failed on uplink (per gateway).",
	}, []string{gatewayID})
)

func uplinkUnknownGateway(g string) prometheus.Counter {
	return ug.With(prometheus.Labels{gatewayID: g})
}

func getGatewayFailed(g string) prometheus.Counter {
	return gg.With(prometheus.Labels{gatewayID: g})
}
