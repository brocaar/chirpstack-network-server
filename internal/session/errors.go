package session

import "errors"

// node-session errors
var (
	ErrDoesNotExistOrFCntOrMICInvalid = errors.New("node-session does not exist or invalid fcnt or mic")
	ErrDoesNotExist                   = errors.New("node-session does not exist")
)
