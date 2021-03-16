package code

import (
	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// MigrateToSchemaMigration This migraton moved gorp_migration to schema_migrations
func MigrateToSchemaMigration(db sqlx.Queryer) error {
	return storage.Transaction(func(tx sqlx.Ext) error {
		var count int
		// gorp_migrations
		err := sqlx.Select(db, &count, `
		select
			count(*)
		from
			gorp_migrations
		`)
		if err != nil {
			return errors.Wrap(err, "select gorp_migrations count error")
		}
		_, err = tx.Exec(`lock table schema_migrations`)
		if err != nil {
			return errors.Wrap(err, "lock code migration table error")
		}

		res, err := tx.Exec(`
			insert into schema_migrations (
				version,
				dirty
			) values ($1, $2)
			on conflict
				do nothing
		`, count, false)
		if err != nil {
			switch err := err.(type) {
			case *pq.Error:
				switch err.Code.Name() {
				case "unique_violation":
					return nil
				}
			}

			return err
		}

		ra, err := res.RowsAffected()
		if err != nil {
			return err
		}

		if ra == 0 {
			return nil
		}

		return nil
	})
	log.Info("gorp_migrations migration to schema_migrations updated")
	return nil
}
