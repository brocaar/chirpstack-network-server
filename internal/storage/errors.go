package storage

import (
	"database/sql"

	"github.com/lib/pq"
	"github.com/pkg/errors"
)

// errors
var (
	ErrAlreadyExists                  = errors.New("object already exists")
	ErrDoesNotExist                   = errors.New("object does not exist")
	ErrDoesNotExistOrFCntOrMICInvalid = errors.New("device-session does not exist or invalid fcnt or mic")
	ErrFCntInvalid                    = errors.New("invalid fcnt")
	ErrInvalidAggregationInterval     = errors.New("invalid aggregation interval")
	ErrInvalidName                    = errors.New("invalid gateway name")
	ErrInvalidFPort                   = errors.New("invalid fPort (must be > 0)")
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
		case "foreign_key_violation":
			return ErrDoesNotExist
		}
	}

	return errors.Wrap(err, description)
}
