package common

import "errors"

// ErrEmptyQueue defines the error returned when the queue is empty.
// Note that depending the context, this error might not be a real error.
var ErrEmptyQueue = errors.New("the queue is empty or does not exist")
