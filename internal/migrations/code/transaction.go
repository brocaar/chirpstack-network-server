package code

import (
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

var errRollback = errors.New("rollback")

// transaction wraps the given function in a transaction. In case the given
// functions returns an error, the transaction will be rolled back.
// Note: this is a copy of storage.Transaction, but since the storage package
// uses the code migrations, using it from there would cause an import cycle.
func transaction(db *sqlx.DB, f func(tx sqlx.Ext) error) error {
	tx, err := db.Beginx()
	if err != nil {
		return errors.Wrap(err, "migration/code: begin transaction error")
	}

	err = f(tx)
	if err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return errors.Wrap(rbErr, "migration/code: transaction rollback error")
		}

		if err == errRollback {
			return nil
		}

		return err
	}

	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, "storage: transaction commit error")
	}
	return nil
}
