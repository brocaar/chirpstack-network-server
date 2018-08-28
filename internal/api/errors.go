package api

import (
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/brocaar/loraserver/internal/downlink/data"
	"github.com/brocaar/loraserver/internal/downlink/multicast"
	"github.com/brocaar/loraserver/internal/downlink/proprietary"
	"github.com/brocaar/loraserver/internal/storage"
)

var errToCode = map[error]codes.Code{
	data.ErrFPortMustNotBeZero:     codes.InvalidArgument,
	data.ErrFPortMustBeZero:        codes.InvalidArgument,
	data.ErrNoLastRXInfoSet:        codes.FailedPrecondition,
	data.ErrInvalidDataRate:        codes.Internal,
	data.ErrMaxPayloadSizeExceeded: codes.InvalidArgument,

	proprietary.ErrInvalidDataRate: codes.Internal,

	multicast.ErrInvalidFCnt: codes.InvalidArgument,

	storage.ErrAlreadyExists:                  codes.AlreadyExists,
	storage.ErrDoesNotExistOrFCntOrMICInvalid: codes.NotFound,
	storage.ErrDoesNotExist:                   codes.NotFound,
	storage.ErrInvalidName:                    codes.InvalidArgument,
	storage.ErrInvalidAggregationInterval:     codes.InvalidArgument,
	storage.ErrInvalidFPort:                   codes.InvalidArgument,
}

func errToRPCError(err error) error {
	cause := errors.Cause(err)
	code, ok := errToCode[cause]
	if !ok {
		code = codes.Unknown
	}
	return grpc.Errorf(code, cause.Error())
}
