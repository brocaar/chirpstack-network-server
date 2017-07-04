package gateway

import "errors"

// gateway errors
var (
	ErrDoesNotExist               = errors.New("gateway does not exist")
	ErrAlreadyExists              = errors.New("gateway already exists")
	ErrInvalidAggregationInterval = errors.New("invalid aggregation interval")
	ErrInvalidName                = errors.New("invalid gateway name")
	ErrInvalidBand                = errors.New("invalid band")
	ErrInvalidChannel             = errors.New("invalid channel")
	ErrInvalidChannelConfig       = errors.New("invalid channel configuration")
	ErrInvalidChannelModulation   = errors.New("invalid channel modulation")
)
