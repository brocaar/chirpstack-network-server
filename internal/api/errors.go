package api

import (
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/brocaar/loraserver/internal/downlink"
)

var errToCode = map[error]codes.Code{
	downlink.ErrFPortMustNotBeZero:     codes.InvalidArgument,
	downlink.ErrFPortMustBeZero:        codes.InvalidArgument,
	downlink.ErrNoLastRXInfoSet:        codes.FailedPrecondition,
	downlink.ErrInvalidDataRate:        codes.Internal,
	downlink.ErrMaxPayloadSizeExceeded: codes.InvalidArgument,
}

func errToRPCError(err error) error {
	cause := errors.Cause(err)
	code, ok := errToCode[cause]
	if !ok {
		code = codes.Unknown
	}
	return grpc.Errorf(code, cause.Error())
}
