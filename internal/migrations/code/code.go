package code

import (
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/pkg/errors"

	"github.com/brocaar/loraserver/internal/storage"
)

// Migrate checks if the given function code has been applied and if not
// it will execute the given function.
func Migrate(name string, f func(db sqlx.Ext) error) error {
	return storage.Transaction(func(tx sqlx.Ext) error {
		_, err := tx.Exec(`lock table code_migration`)
		if err != nil {
			return errors.Wrap(err, "lock code migration table error")
		}

		res, err := tx.Exec(`
			insert into code_migration (
				id,
				applied_at
			) values ($1, $2)
			on conflict
				do nothing
		`, name, time.Now())
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

		return f(tx)
	})
}
