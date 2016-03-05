package loraserver

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestValidateDevNonce(t *testing.T) {
	Convey("Given a Node", t, func() {
		n := Node{
			UsedDevNonces: make([][2]byte, 0),
		}

		Convey("Then any given dev-nonce is valid", func() {
			dn := [2]byte{1, 2}
			So(n.ValidateDevNonce(dn), ShouldBeTrue)

			Convey("Then the dev-nonce is added to the used nonces", func() {
				So(n.UsedDevNonces, ShouldContain, dn)
			})
		})

		Convey("Given a Node has 10 used nonces", func() {
			n.UsedDevNonces = [][2]byte{
				{1, 1},
				{2, 2},
				{3, 3},
				{4, 4},
				{5, 5},
				{6, 6},
				{7, 7},
				{8, 8},
				{9, 9},
				{10, 10},
			}

			Convey("Then an already used nonce is invalid", func() {
				So(n.ValidateDevNonce([2]byte{1, 1}), ShouldBeFalse)

				Convey("Then the UsedDevNonces still has length 10", func() {
					So(n.UsedDevNonces, ShouldHaveLength, 10)
				})
			})

			Convey("Then a new nonce is valid", func() {
				So(n.ValidateDevNonce([2]byte{0, 0}), ShouldBeTrue)

				Convey("Then the UsedDevNonces still has length 10", func() {
					So(n.UsedDevNonces, ShouldHaveLength, 10)
				})

				Convey("Then the new nonce was added to the back of the list", func() {
					So(n.UsedDevNonces[9], ShouldEqual, [2]byte{0, 0})
				})
			})
		})
	})
}
