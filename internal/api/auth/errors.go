package auth

import "errors"

// errors
var (
	ErrNoMetadataInContext       = errors.New("no metadata in context")
	ErrNoAuthorizationInMetadata = errors.New("no authorization-data in metadata")
	ErrInvalidAlgorithm          = errors.New("invalid algorithm")
	ErrInvalidToken              = errors.New("invalid token")
)
