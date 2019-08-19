package azureiothub

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	ec = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "backend_azure_iot_hub_event_count",
		Help: "The number of received events by the Azure IoT Hub backend (per event type).",
	}, []string{"event"})

	cc = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "backend_azure_iot_hub_command_count",
		Help: "The number of published commands by the Azure IoT Hub backend (per command type).",
	}, []string{"command"})

	cr = promauto.NewCounter(prometheus.CounterOpts{
		Name: "backend_azure_connection_recover_count",
	})
)

func azureEventCounter(e string) prometheus.Counter {
	return ec.With(prometheus.Labels{"event": e})
}

func azureCommandCounter(c string) prometheus.Counter {
	return cc.With(prometheus.Labels{"command": c})
}

func azureConnectionRecoverCounter() prometheus.Counter {
	return cr
}
