package api

import (
	"github.com/brocaar/loraserver/internal/api/auth"
	"golang.org/x/net/context"
)

type TestValidator struct {
	ctx            context.Context
	validatorFuncs []auth.ValidatorFunc
	returnError    error
}

func (v *TestValidator) Validate(ctx context.Context, funcs ...auth.ValidatorFunc) error {
	v.ctx = ctx
	v.validatorFuncs = funcs
	return v.returnError
}
