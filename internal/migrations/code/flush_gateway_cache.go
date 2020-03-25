package code

import (
	"context"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/brocaar/lorawan"
)

// FlushGatewayCache flushes the gateway cache.
func FlushGatewayCache(db sqlx.Ext) error {
	var ids []lorawan.EUI64

	err := sqlx.Select(db, &ids, `
		select
			gateway_id
		from
			gateway
	`)
	if err != nil {
		return errors.Wrap(err, "select gateway ids error")
	}

	for _, id := range ids {
		if err := storage.FlushGatewayCache(context.Background(), id); err != nil {
			log.WithError(err).Error("flush gateway cache error")
		}
	}

	return nil
}
