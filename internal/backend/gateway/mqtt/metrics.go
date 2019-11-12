package mqtt

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	ec = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "backend_mqtt_event_count",
		Help: "The number of received events by the MQTT backend (per event type).",
	}, []string{"event"})

	cc = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "backend_mqtt_command_count",
		Help: "The number of published commands by the MQTT backend (per command).",
	}, []string{"command"})

	mqttc = promauto.NewCounter(prometheus.CounterOpts{
		Name: "backend_mqtt_connect_count",
		Help: "The number of times the MQTT backend connected to the MQTT broker.",
	})

	mqttd = promauto.NewCounter(prometheus.CounterOpts{
		Name: "backend_mqtt_disconnect_count",
		Help: "The number of times the MQTT backend disconnected from the MQTT broker.",
	})
)

func mqttEventCounter(e string) prometheus.Counter {
	return ec.With(prometheus.Labels{"event": e})
}

func mqttCommandCounter(c string) prometheus.Counter {
	return cc.With(prometheus.Labels{"command": c})
}

func mqttConnectCounter() prometheus.Counter {
	return mqttc
}

func mqttDisconnectCounter() prometheus.Counter {
	return mqttd
}
