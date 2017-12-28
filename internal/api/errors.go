package api

import (
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/brocaar/loraserver/internal/downlink/data"
	"github.com/brocaar/loraserver/internal/downlink/proprietary"
	"github.com/brocaar/loraserver/internal/gateway"
	"github.com/brocaar/loraserver/internal/storage"
)

var errToCode = map[error]codes.Code{
	data.ErrFPortMustNotBeZero:     codes.InvalidArgument,
	data.ErrFPortMustBeZero:        codes.InvalidArgument,
	data.ErrNoLastRXInfoSet:        codes.FailedPrecondition,
	data.ErrInvalidDataRate:        codes.Internal,
	data.ErrMaxPayloadSizeExceeded: codes.InvalidArgument,

	proprietary.ErrInvalidDataRate: codes.Internal,

	gateway.ErrDoesNotExist:               codes.NotFound,
	gateway.ErrAlreadyExists:              codes.AlreadyExists,
	gateway.ErrInvalidAggregationInterval: codes.InvalidArgument,
	gateway.ErrInvalidName:                codes.InvalidArgument,
	gateway.ErrInvalidBand:                codes.InvalidArgument,
	gateway.ErrInvalidChannel:             codes.InvalidArgument,
	gateway.ErrInvalidChannelConfig:       codes.InvalidArgument,
	gateway.ErrInvalidChannelModulation:   codes.InvalidArgument,

	storage.ErrDoesNotExistOrFCntOrMICInvalid: codes.NotFound,
	storage.ErrDoesNotExist:                   codes.NotFound,
}

func errToRPCError(err error) error {
	cause := errors.Cause(err)
	code, ok := errToCode[cause]
	if !ok {
		code = codes.Unknown
	}
	return grpc.Errorf(code, cause.Error())
}
