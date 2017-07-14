package auth

import (
	"context"
	"fmt"
	"testing"
	"time"

	"google.golang.org/grpc/metadata"

	"github.com/brocaar/lorawan"
	jwt "github.com/dgrijalva/jwt-go"
	"github.com/pkg/errors"
	. "github.com/smartystreets/goconvey/convey"
)

func TestJWTValidator(t *testing.T) {
	Convey("Given a JWT validator", t, func() {
		v := JWTValidator{
			secret:    "verysecret",
			algorithm: "HS256",
		}

		testTable := []struct {
			Description   string
			Key           string
			Claims        Claims
			ExpectedMAC   lorawan.EUI64
			ExpectedError string
		}{
			{
				Description: "valid key and passing validation",
				Key:         v.secret,
				Claims:      Claims{MAC: lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}},
				ExpectedMAC: lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
			},
			{
				Description:   "valid key and expired token",
				Key:           v.secret,
				Claims:        Claims{MAC: lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}, StandardClaims: jwt.StandardClaims{ExpiresAt: time.Now().Unix() - 1}},
				ExpectedError: "token is expired by 1s",
			},
			{
				Description:   "invalid key",
				Key:           "differentsecret",
				Claims:        Claims{},
				ExpectedError: "signature is invalid",
			},
		}

		for i, test := range testTable {
			Convey(fmt.Sprintf("Test %s [%d]", test.Description, i), func() {
				token := jwt.NewWithClaims(jwt.SigningMethodHS256, test.Claims)
				ss, err := token.SignedString([]byte(test.Key))
				So(err, ShouldBeNil)

				ctx := context.Background()
				ctx = metadata.NewIncomingContext(ctx, metadata.MD{
					"authorization": []string{ss},
				})

				mac, err := v.GetMAC(ctx)
				if test.ExpectedError != "" {
					So(errors.Cause(err).Error(), ShouldResemble, test.ExpectedError)
				} else {
					So(mac, ShouldResemble, test.ExpectedMAC)
				}
			})
		}
	})
}
