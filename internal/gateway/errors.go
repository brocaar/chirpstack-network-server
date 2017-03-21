package gateway

import "errors"

// gateway errors
var (
	ErrDoesNotExist               = errors.New("gateway does not exist")
	ErrAlreadyExists              = errors.New("gateway already exists")
	ErrInvalidAggregationInterval = errors.New("invalid aggregation interval")
)
