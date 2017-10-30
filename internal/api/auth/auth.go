package auth

import (
	"context"
	"fmt"

	"google.golang.org/grpc/metadata"

	"github.com/brocaar/lorawan"
	jwt "github.com/dgrijalva/jwt-go"
	"github.com/pkg/errors"
)

// Claims define the struct containing the token claims.
type Claims struct {
	jwt.StandardClaims

	// MAC defines the gateway mac.
	MAC lorawan.EUI64 `json:"mac"`
}

// Validator defines the interface that a validator needs to implement.
type Validator interface {
	// GetMAC returns the MAC of the authenticated gateway.
	GetMAC(context.Context) (lorawan.EUI64, error)
}

// JWTValidator validates JWT tokens.
type JWTValidator struct {
	secret    string
	algorithm string
}

// NewJWTValidator creates a new JWTValidator.
func NewJWTValidator(algorithm, secret string) *JWTValidator {
	return &JWTValidator{
		secret:    secret,
		algorithm: algorithm,
	}
}

// GetMAC returns the gateway MAC from the JWT claim.
func (v JWTValidator) GetMAC(ctx context.Context) (lorawan.EUI64, error) {
	claims, err := v.getClaims(ctx)
	if err != nil {
		return lorawan.EUI64{}, err
	}

	return claims.MAC, nil
}

func (v JWTValidator) getClaims(ctx context.Context) (*Claims, error) {
	tokenStr, err := getTokenFromContext(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "get token from context error")
	}

	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if token.Header["alg"] != v.algorithm {
			return nil, ErrInvalidAlgorithm
		}
		return []byte(v.secret), nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "jwt parse error")
	}

	if !token.Valid {
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*Claims)
	if !ok {
		// no need to use a static error, this should never happen
		return nil, fmt.Errorf("api/auth: expected *Claims, got %T", token.Claims)
	}

	return claims, nil
}

func getTokenFromContext(ctx context.Context) (string, error) {
	md, ok := metadata.FromContext(ctx)
	if !ok {
		return "", ErrNoMetadataInContext
	}

	token, ok := md["authorization"]
	if !ok || len(token) == 0 {
		return "", ErrNoAuthorizationInMetadata
	}

	return token[0], nil
}
