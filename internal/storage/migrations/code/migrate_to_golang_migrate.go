package code

import (
	"strconv"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// MigrateToGolangMigrate migrates the gorp_migrations into schema_migrations.
func MigrateToGolangMigrate(db sqlx.Ext) error {
	var maxID string

	_, err := db.Exec(`savepoint before_gorp`)
	if err != nil {
		return errors.Wrap(err, "savepoint error")
	}

	// Get max migration id.
	err = sqlx.Get(db, &maxID, `
		select
			id
		from
			gorp_migrations
		order by
			applied_at desc
		limit 1
	`)
	if err != nil {
		switch err := err.(type) {
		case *pq.Error:
			switch err.Code.Name() {
			case "undefined_table":
				// gorp_migrations table does not exist, this is a clean install and
				// there is nothing to migrate.

				// rollback to before savepoint as we can not commit a failed transaction
				_, err := db.Exec(`rollback to savepoint before_gorp`)
				if err != nil {
					return errors.Wrap(err, "savepoint rollback error")
				}

				return nil
			}
		}

		return errors.Wrap(err, "select max migration id error")
	}

	// Split the 0001_foo_bar.sql format into an int
	maxIDParts := strings.Split(maxID, "_")
	version, err := strconv.ParseInt(maxIDParts[0], 10, 64)
	if err != nil {
		return errors.Wrap(err, "parse migration id to int error")
	}

	log.WithFields(log.Fields{
		"gorp_migrations_id":               maxID,
		"golang_schema_migrations_version": version,
	}).Info("migrations/code: migrating from gorp_migrations to schema_migrations")

	// Create the schema_migrations table
	_, err = db.Exec(`
		create table if not exists schema_migrations (
			version bigint primary key not null,
			dirty boolean not null
	)`)
	if err != nil {
		return errors.Wrap(err, "create schema_migrations table error")
	}

	// Insert the version into schema_migrations
	_, err = db.Exec(`
		insert into schema_migrations (
			version,
			dirty
		) values ($1, $2)`,
		version,
		false,
	)
	if err != nil {
		return errors.Wrap(err, "insert migration error")
	}

	return nil
}
