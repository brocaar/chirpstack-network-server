package amqp

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	ec = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "backend_amqp_event_count",
		Help: "The number of received events by the AMQP / RabbitMQ backend (per event type).",
	}, []string{"event"})

	cc = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "backend_amqp_command_count",
		Help: "The number of published commands by the AMQP / RabbitMQ backend (per command).",
	}, []string{"command"})
)

func amqpEventCounter(e string) prometheus.Counter {
	return ec.With(prometheus.Labels{"event": e})
}

func amqpCommandCounter(c string) prometheus.Counter {
	return cc.With(prometheus.Labels{"command": c})
}
