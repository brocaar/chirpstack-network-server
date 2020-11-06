package join

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	gd = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "join_object_not_found_count",
		Help: "The number of join requests where the device was not found (per deveui).",
	}, []string{"deveui"})
	dn = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "validate_dev_nonce_error_count",
		Help: "The number of join requests where the dev nonce validation errored (per deveui).",
	}, []string{"deveui"})
)

func getDeviceNotExist(d string) prometheus.Counter {
	return gd.With(prometheus.Labels{"deveui": d})
}

func validateDevNonce(d string) prometheus.Counter {
	return dn.With(prometheus.Labels{"deveui": d})
}
