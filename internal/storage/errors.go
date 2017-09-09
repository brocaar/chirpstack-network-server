package storage

import (
	"database/sql"

	"github.com/lib/pq"
	"github.com/pkg/errors"
)

// errors
var (
	ErrAlreadyExists = errors.New("object already exists")
	ErrDoesNotExist  = errors.New("object does not exist")
)

func handlePSQLError(err error, description string) error {
	if err == sql.ErrNoRows {
		return ErrDoesNotExist
	}

	switch err := err.(type) {
	case *pq.Error:
		switch err.Code.Name() {
		case "unique_violation":
			return ErrAlreadyExists
		}
	}

	return errors.Wrap(err, description)
}
