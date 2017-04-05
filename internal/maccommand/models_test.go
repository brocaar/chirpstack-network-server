package maccommand

import (
	"testing"

	"github.com/brocaar/lorawan"
	. "github.com/smartystreets/goconvey/convey"
)

func TestMACCommands(t *testing.T) {
	Convey("Given a MACCommands instance", t, func() {
		macCommands := MACCommands{
			{
				CID:     lorawan.RXParamSetupReq,
				Payload: &lorawan.RX2SetupReqPayload{Frequency: 868100000},
			},
		}

		Convey("Then UnmarshalBinary returns the same item after MarshalBinary", func() {
			b, err := macCommands.MarshalBinary()
			So(err, ShouldBeNil)

			var mc MACCommands
			So(mc.UnmarshalBinary(b), ShouldBeNil)
			So(mc, ShouldResemble, macCommands)
		})
	})
}
