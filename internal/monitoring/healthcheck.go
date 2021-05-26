package monitoring

import (
	"context"
	"net/http"

	"github.com/brocaar/chirpstack-network-server/v3/internal/storage"
	"github.com/pkg/errors"
)

func healthCheckHandlerFunc(w http.ResponseWriter, r *http.Request) {
	_, err := storage.RedisClient().Ping(context.Background()).Result()
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(errors.Wrap(err, "redis ping error").Error()))
	}

	err = storage.DB().Ping()
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(errors.Wrap(err, "postgresql ping error").Error()))
	}

	w.WriteHeader(http.StatusOK)
}
