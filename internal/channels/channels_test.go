package channels

import (
	"fmt"
	"testing"

	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/brocaar/chirpstack-network-server/internal/test"
	"github.com/brocaar/lorawan"
	. "github.com/smartystreets/goconvey/convey"
)

func TestHandleChannelReconfigure(t *testing.T) {
	_ = test.GetConfig()

	Convey("Given a set of tests", t, func() {
		tests := []struct {
			Name          string
			DeviceSession storage.DeviceSession
			Expected      []storage.MACCommandBlock
		}{
			{
				Name: "no channels to reconfigure",
				DeviceSession: storage.DeviceSession{
					TXPowerIndex:          1,
					NbTrans:               2,
					EnabledUplinkChannels: []int{0, 1, 2},
				},
			},
			{
				Name: "channels to reconfigure",
				DeviceSession: storage.DeviceSession{
					TXPowerIndex:          1,
					NbTrans:               2,
					EnabledUplinkChannels: []int{0, 1}, // this is not realistic but good enough for testing
					DR: 3,
				},
				Expected: []storage.MACCommandBlock{
					{
						CID: lorawan.LinkADRReq,
						MACCommands: storage.MACCommands{
							lorawan.MACCommand{
								CID: lorawan.LinkADRReq,
								Payload: &lorawan.LinkADRReqPayload{
									DataRate: 3,
									TXPower:  1,
									ChMask:   lorawan.ChMask{true, true, true},
									Redundancy: lorawan.Redundancy{
										NbRep: 2,
									},
								},
							},
						},
					},
				},
			},
		}

		for i, test := range tests {
			Convey(fmt.Sprintf("test: %s [%d]", test.Name, i), func() {
				blocks, err := HandleChannelReconfigure(test.DeviceSession)
				So(err, ShouldBeNil)
				So(blocks, ShouldResemble, test.Expected)
			})
		}
	})
}
