package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/brocaar/lorawan"
	jwt "github.com/dgrijalva/jwt-go"
	. "github.com/smartystreets/goconvey/convey"
)

func TestValidateApplication(t *testing.T) {
	Convey("Given a test table", t, func() {
		testTable := []struct {
			Description string
			AppEUI      lorawan.EUI64
			Claims      Claims
			Error       error
		}{
			{
				Description: "User is admin",
				AppEUI:      [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
				Claims:      Claims{Admin: true},
				Error:       nil,
			},
			{
				Description: "User has access to all applications",
				AppEUI:      [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
				Claims:      Claims{Applications: []string{"*"}},
				Error:       nil,
			},
			{
				Description: "User has access specific application",
				AppEUI:      [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
				Claims:      Claims{Applications: []string{"0102030405060708"}},
				Error:       nil,
			},
			{
				Description: "User has no permission to application",
				AppEUI:      [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
				Claims:      Claims{Applications: []string{"0807060504030201"}},
				Error:       errors.New("no permission to application 0102030405060708"),
			},
		}

		for _, test := range testTable {
			Convey("Test: "+test.Description, func() {
				v := ValidateApplication(test.AppEUI)
				So(v(&test.Claims), ShouldResemble, test.Error)
			})
		}
	})
}

func TestValidateNode(t *testing.T) {
	Convey("Given a test table", t, func() {
		testTable := []struct {
			Description string
			DevEUI      lorawan.EUI64
			Claims      Claims
			Error       error
		}{
			{
				Description: "User is admin",
				DevEUI:      [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
				Claims:      Claims{Admin: true},
				Error:       nil,
			},
			{
				Description: "User has access to all nodes",
				DevEUI:      [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
				Claims:      Claims{Nodes: []string{"*"}},
				Error:       nil,
			},
			{
				Description: "User has access specific node",
				DevEUI:      [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
				Claims:      Claims{Nodes: []string{"0102030405060708"}},
				Error:       nil,
			},
			{
				Description: "User has no permission to node",
				DevEUI:      [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
				Claims:      Claims{Nodes: []string{"0807060504030201"}},
				Error:       errors.New("no permission to node 0102030405060708"),
			},
		}

		for _, test := range testTable {
			Convey("Test: "+test.Description, func() {
				v := ValidateNode(test.DevEUI)
				So(v(&test.Claims), ShouldResemble, test.Error)
			})
		}
	})
}

func TestValidateAPIMethod(t *testing.T) {
	Convey("Given a test table", t, func() {
		testTable := []struct {
			Description string
			APIMethod   string
			Claims      Claims
			Error       error
		}{
			{
				Description: "User is admin",
				APIMethod:   "TestAPI.TestMethod",
				Claims:      Claims{Admin: true},
				Error:       nil,
			},
			{
				Description: "User has access to all methods",
				APIMethod:   "TestAPI.TestMethod",
				Claims:      Claims{APIMethods: []string{"*"}},
				Error:       nil,
			},
			{
				Description: "User has access to all methods of same API",
				APIMethod:   "TestAPI.TestMethod",
				Claims:      Claims{APIMethods: []string{"TestAPI.*"}},
				Error:       nil,
			},
			{
				Description: "User has access to specific method",
				APIMethod:   "TestAPI.TestMethod",
				Claims:      Claims{APIMethods: []string{"TestAPI.TestMethod"}},
				Error:       nil,
			},
			{
				Description: "User has access to multiple methods of same API",
				APIMethod:   "TestAPI.TestMethod",
				Claims:      Claims{APIMethods: []string{"TestAPI.(TestMethod|OtherTestMethod)"}},
				Error:       nil,
			},
			{
				Description: "User doesn't have access to api method",
				APIMethod:   "TestAPI.TestMethod",
				Claims:      Claims{APIMethods: []string{"API.(TestMethod|OtherTestMethod)", "API.*", "API.TestMethod"}},
				Error:       errors.New("no permission to api method: TestAPI.TestMethod"),
			},
		}

		for _, test := range testTable {
			Convey("Test: "+test.Description, func() {
				v := ValidateAPIMethod(test.APIMethod)
				So(v(&test.Claims), ShouldResemble, test.Error)
			})
		}
	})
}

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
			ValidatorFunc ValidatorFunc
			Error         error
		}{
			{
				Description:   "valid key and passing validation",
				Key:           v.secret,
				Claims:        Claims{Admin: true},
				ValidatorFunc: ValidateNode([8]byte{1, 2, 3, 4, 5, 6, 7, 8}),
				Error:         nil,
			},
			{
				Description:   "valid key and expired token",
				Key:           v.secret,
				Claims:        Claims{Admin: true, Expiration: time.Now().Unix() - 1},
				ValidatorFunc: ValidateNode([8]byte{1, 2, 3, 4, 5, 6, 7, 8}),
				Error:         errors.New("api/auth: jwt parse error: token has expired"),
			},
			{
				Description:   "invalid key",
				Key:           "differentsecret",
				Claims:        Claims{Admin: true},
				ValidatorFunc: ValidateNode([8]byte{1, 2, 3, 4, 5, 6, 7, 8}),
				Error:         errors.New("api/auth: jwt parse error: signature is invalid"),
			},
			{
				Description:   "valid key but failing validation",
				Key:           v.secret,
				Claims:        Claims{},
				ValidatorFunc: ValidateNode([8]byte{1, 2, 3, 4, 5, 6, 7, 8}),
				Error:         errors.New("auth/api: no permission to node 0102030405060708"),
			},
		}

		for _, test := range testTable {
			Convey("Test: "+test.Description, func() {
				token := jwt.NewWithClaims(jwt.SigningMethodHS256, test.Claims)
				ss, err := token.SignedString([]byte(test.Key))
				So(err, ShouldBeNil)

				So(v.Validate(ss, test.ValidatorFunc), ShouldResemble, test.Error)
			})
		}
	})
}
