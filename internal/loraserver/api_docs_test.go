package loraserver

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGetRPCServicesDoc(t *testing.T) {
	Convey("Given a RPC service", t, func() {
		api := NewApplicationAPI(Context{})

		Convey("Then all expected strings are in the returned JSON", func() {
			doc, err := getRPCServicesDoc(api)
			So(err, ShouldBeNil)

			Convey("Then the doc has an Application entry", func() {
				app, ok := doc["Application"]
				So(ok, ShouldBeTrue)

				Convey("Which has a Create method", func() {
					meth, ok := app.Methods["Create"]
					So(ok, ShouldBeTrue)

					Convey("Which has the expected argument and reply", func() {
						So(meth.ArgumentTypeName, ShouldEqual, "Application")
						So(meth.ArgumentPkgPath, ShouldEqual, "github.com/brocaar/loraserver/models")
						So(meth.ArgumentJSON, ShouldContainSubstring, "appEUI")
						So(meth.ArgumentJSON, ShouldContainSubstring, "Application.Create")
						So(meth.ReplyTypeName, ShouldEqual, "EUI64")
						So(meth.ReplyPkgPath, ShouldEqual, "github.com/brocaar/lorawan")
						So(meth.ReplyJSON, ShouldContainSubstring, "0000000000000000")
					})
				})
			})
		})
	})
}
