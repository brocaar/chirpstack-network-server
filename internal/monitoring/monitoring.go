package monitoring

import (
	"net/http"

	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/chirpstack-network-server/internal/config"
)

// Setup setsup the metrics server.
func Setup(c config.Config) error {
	if c.Monitoring.Bind != "" {
		return setupNew(c)
	}
	return setupLegacy(c)
}

func setupNew(c config.Config) error {
	if c.Monitoring.Bind == "" {
		return nil
	}

	log.WithFields(log.Fields{
		"bind": c.Monitoring.Bind,
	}).Info("monitoring: setting up monitoring endpoint")

	mux := http.NewServeMux()

	if c.Monitoring.PrometheusAPITimingHistogram {
		log.Info("monitoring: enabling Prometheus api timing histogram")
		grpc_prometheus.EnableHandlingTimeHistogram()
	}

	if c.Monitoring.PrometheusEndpoint {
		log.WithFields(log.Fields{
			"endpoint": "/metrics",
		}).Info("monitoring: registering Prometheus endpoint")
		mux.Handle("/metrics", promhttp.Handler())
	}

	if c.Monitoring.HealthcheckEndpoint {
		log.WithFields(log.Fields{
			"endpoint": "/healthcheck",
		}).Info("monitoring: registering healthcheck endpoint")
		mux.HandleFunc("/health", healthCheckHandlerFunc)
	}

	server := http.Server{
		Handler: mux,
		Addr:    c.Monitoring.Bind,
	}

	go func() {
		err := server.ListenAndServe()
		log.WithError(err).Error("monitoring: monitoring server error")
	}()

	return nil
}

func setupLegacy(c config.Config) error {
	if !c.Metrics.Prometheus.EndpointEnabled {
		return nil
	}

	if c.Metrics.Prometheus.APITimingHistogram {
		grpc_prometheus.EnableHandlingTimeHistogram()
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
