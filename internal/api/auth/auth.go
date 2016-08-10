package auth

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/net/context"
	"google.golang.org/grpc/metadata"

	"github.com/brocaar/lorawan"
	jwt "github.com/dgrijalva/jwt-go"
)

// Claims defines the struct containing the token claims.
type Claims struct {
	jwt.StandardClaims

	// Admin defines if the user has admin permissions. If true, the user has
	// all permissions.
	Admin bool `json:"admin"`

	// APIMethods defines the API methods the user has access to.
	// The following forms are allowed:
	// All methods: ["*"]
	// One Application method: ["Application.Create"]
	// Multiple Application methods: ["Application.(Create|Delete)"]
	// All Application methods: ["Application.*"]
	APIMethods []string `json:"apis"`

	// Applications defines the applications the user has access to.
	// The following forms are allowed:
	// All applications: ["*"]
	// One or multiple applications: ["0102030405060708", ...]
	Applications []string `json:"apps"`

	// Nodes defines the nodes the user has access to. It follows the same
	// logic as the applications.
	Nodes []string `json:"nodes"`
}

// Validator defines the interface a validator needs to implement.
type Validator interface {
	Validate(context.Context, ...ValidatorFunc) error
}

// ValidatorFunc defines the signature of a claim validator function.
type ValidatorFunc func(*Claims) error

// NopValidator doesn't perform any validation and returns alway true.
type NopValidator struct{}

// Validate validates the given token against the given validator funcs.
// In the case of the NopValidator, it returns always nil.
func (v NopValidator) Validate(ctx context.Context, funcs ...ValidatorFunc) error {
	return nil
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

// Validate validates the token from the given context against the given
// validator funcs.
func (v JWTValidator) Validate(ctx context.Context, funcs ...ValidatorFunc) error {
	tokenStr, err := getTokenFromContext(ctx)
	if err != nil {
		return err
	}

	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if token.Header["alg"] != v.algorithm {
			return nil, fmt.Errorf("api/auth: unexpected algorithm %s, expected %s", token.Header["alg"], v.algorithm)
		}
		return []byte(v.secret), nil
	})
	if err != nil {
		return fmt.Errorf("api/auth: jwt parse error: %s", err)
	}

	if !token.Valid {
		return errors.New("api/auth: invalid token")
	}

	claims, ok := token.Claims.(*Claims)
	if !ok {
		return fmt.Errorf("api/auth: expected *Claims, got %T", token.Claims)
	}

	for _, f := range funcs {
		if err := f(claims); err != nil {
			return fmt.Errorf("auth/api: %s", err)
		}
	}

	return nil
}

// ValidateApplication validates if the user has permission to the given AppEUI.
func ValidateApplication(appEUI lorawan.EUI64) ValidatorFunc {
	return func(claims *Claims) error {
		if claims.Admin {
			return nil
		}

		for _, app := range claims.Applications {
			if app == "*" {
				return nil
			}

			if appEUI.String() == app {
				return nil
			}
		}

		return fmt.Errorf("no permission to application %s", appEUI)
	}
}

// ValidateNode validates if the user has permission to the given DevEUI.
func ValidateNode(devEUI lorawan.EUI64) ValidatorFunc {
	return func(claims *Claims) error {
		if claims.Admin {
			return nil
		}

		for _, node := range claims.Nodes {
			if node == "*" {
				return nil
			}

			if devEUI.String() == node {
				return nil
			}
		}

		return fmt.Errorf("no permission to node %s", devEUI)
	}
}

// ValidateAPIMethod validates if the user has permission to the given api method.
func ValidateAPIMethod(apiMethod string) ValidatorFunc {
	return func(claims *Claims) error {
		methodParts := strings.SplitN(apiMethod, ".", 2)
		if len(methodParts) != 2 {
			return fmt.Errorf("invalid api method: %s", apiMethod)
		}

		if claims.Admin {
			return nil
		}

		for _, meth := range claims.APIMethods {
			if meth == "*" {
				return nil
			}

			if apiMethod == meth {
				return nil
			}

			if methodParts[0]+".*" == meth {
				return nil
			}

			if match, err := regexp.MatchString("^"+strings.Replace(meth, ".", `\.`, -1)+"$", apiMethod); err != nil {
				return fmt.Errorf("regexp match error on %s: %s", meth, err)
			} else if match {
				return nil
			}
		}

		return fmt.Errorf("no permission to api method: %s", apiMethod)
	}
}

func getTokenFromContext(ctx context.Context) (string, error) {
	md, ok := metadata.FromContext(ctx)
	if !ok {
		return "", errors.New("could not get metadata from context")
	}

	token, ok := md["authorization"]
	if !ok || len(token) == 0 {
		return "", errors.New("authorization missing in metadata")
	}

	return token[0], nil
}
