package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/loraserver/internal/config"
)

// Setup setsup the metrics server.
func Setup(c config.Config) error {
	if !c.Metrics.Prometheus.EndpointEnabled {
		return nil
	}

	log.WithFields(log.Fields{
		"bind": c.Metrics.Prometheus.Bind,
	}).Info("metrics: starting prometheus metrics server")

	server := http.Server{
		Handler: promhttp.Handler(),
		Addr:    c.Metrics.Prometheus.Bind,
	}

	go func() {
		err := server.ListenAndServe()
		log.WithError(err).Error("metrics: prometheus metrics server error")
	}()

	return nil
}
