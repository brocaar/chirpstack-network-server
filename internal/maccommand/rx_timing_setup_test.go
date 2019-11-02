package maccommand

import (
	"context"
	"fmt"
	"testing"

	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/brocaar/lorawan"
	. "github.com/smartystreets/goconvey/convey"
)

func TestRequestRXTimingSetup(t *testing.T) {
	Convey("When calling RequestRXTimingSetup", t, func() {
		block := RequestRXTimingSetup(14)

		Convey("Then the expected block is returned", func() {
			So(block, ShouldResemble, storage.MACCommandBlock{
				CID: lorawan.RXTimingSetupReq,
				MACCommands: []lorawan.MACCommand{
					{
						CID: lorawan.RXTimingSetupReq,
						Payload: &lorawan.RXTimingSetupReqPayload{
							Delay: 14,
						},
					},
				},
			})
		})
	})
}

func TestHandleRXTimingSetupAns(t *testing.T) {
	Convey("Given a set of tests", t, func() {
		tests := []struct {
			Name                    string
			DeviceSession           storage.DeviceSession
			ReceivedMACCommandBlock storage.MACCommandBlock
			PendingMACCommandBlock  *storage.MACCommandBlock
			ExpectedDeviceSession   storage.DeviceSession
			ExpectedError           error
		}{
			{
				Name: "rx timing setup ack",
				DeviceSession: storage.DeviceSession{
					RXDelay: 4,
				},
				ReceivedMACCommandBlock: storage.MACCommandBlock{
					CID: lorawan.RXTimingSetupAns,
				},
				PendingMACCommandBlock: &storage.MACCommandBlock{
					CID: lorawan.RXTimingSetupReq,
					MACCommands: []lorawan.MACCommand{
						{
							CID: lorawan.RXTimingSetupReq,
							Payload: &lorawan.RXTimingSetupReqPayload{
								Delay: 14,
							},
						},
					},
				},
				ExpectedDeviceSession: storage.DeviceSession{
					RXDelay: 14,
				},
			},
		}

		for i, t := range tests {
			Convey(fmt.Sprintf("Testing: %s [%d]", t.Name, i), func() {
				ans, err := handleRXTimingSetupAns(context.Background(), &t.DeviceSession, t.ReceivedMACCommandBlock, t.PendingMACCommandBlock)
				So(err, ShouldResemble, t.ExpectedError)
				So(ans, ShouldBeNil)
				So(t.DeviceSession, ShouldResemble, t.ExpectedDeviceSession)
			})
		}
	})
}
