package maccommand

import (
	"context"
	"fmt"
	"testing"

	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/brocaar/lorawan"
	"github.com/pkg/errors"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRequestRejoinParamSetup(t *testing.T) {
	Convey("When calling RequestRejoinParamSetup", t, func() {
		block := RequestRejoinParamSetup(5, 10)

		Convey("Then the expected block is returned", func() {
			So(block, ShouldResemble, storage.MACCommandBlock{
				CID: lorawan.RejoinParamSetupReq,
				MACCommands: []lorawan.MACCommand{
					{
						CID: lorawan.RejoinParamSetupReq,
						Payload: &lorawan.RejoinParamSetupReqPayload{
							MaxTimeN:  5,
							MaxCountN: 10,
						},
					},
				},
			})
		})
	})
}

func TestHandleRejoinParamSetupAns(t *testing.T) {

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
				Name: "acknowledged with time ok",
				DeviceSession: storage.DeviceSession{
					RejoinRequestMaxCountN: 1,
					RejoinRequestMaxTimeN:  2,
				},
				ReceivedMACCommandBlock: storage.MACCommandBlock{
					CID: lorawan.RejoinParamSetupAns,
					MACCommands: []lorawan.MACCommand{
						{
							CID: lorawan.RejoinParamSetupAns,
							Payload: &lorawan.RejoinParamSetupAnsPayload{
								TimeOK: true,
							},
						},
					},
				},
				PendingMACCommandBlock: &storage.MACCommandBlock{
					CID: lorawan.RejoinParamSetupReq,
					MACCommands: []lorawan.MACCommand{
						{
							CID: lorawan.RejoinParamSetupReq,
							Payload: &lorawan.RejoinParamSetupReqPayload{
								MaxCountN: 10,
								MaxTimeN:  5,
							},
						},
					},
				},
				ExpectedDeviceSession: storage.DeviceSession{
					RejoinRequestEnabled:   true,
					RejoinRequestMaxCountN: 10,
					RejoinRequestMaxTimeN:  5,
				},
			},
			{
				// this still is handled as an ACK, only with a printed warning.
				Name: "acknowledged with time not ok",
				DeviceSession: storage.DeviceSession{
					RejoinRequestMaxCountN: 1,
					RejoinRequestMaxTimeN:  2,
				},
				ReceivedMACCommandBlock: storage.MACCommandBlock{
					CID: lorawan.RejoinParamSetupAns,
					MACCommands: []lorawan.MACCommand{
						{
							CID: lorawan.RejoinParamSetupAns,
							Payload: &lorawan.RejoinParamSetupAnsPayload{
								TimeOK: false,
							},
						},
					},
				},
				PendingMACCommandBlock: &storage.MACCommandBlock{
					CID: lorawan.RejoinParamSetupReq,
					MACCommands: []lorawan.MACCommand{
						{
							CID: lorawan.RejoinParamSetupReq,
							Payload: &lorawan.RejoinParamSetupReqPayload{
								MaxCountN: 10,
								MaxTimeN:  5,
							},
						},
					},
				},
				ExpectedDeviceSession: storage.DeviceSession{
					RejoinRequestEnabled:   true,
					RejoinRequestMaxCountN: 10,
					RejoinRequestMaxTimeN:  5,
				},
			},
			{
				Name: "acknowledged, but nothing pending",
				DeviceSession: storage.DeviceSession{
					RejoinRequestMaxCountN: 1,
					RejoinRequestMaxTimeN:  2,
				},
				ReceivedMACCommandBlock: storage.MACCommandBlock{
					CID: lorawan.RejoinParamSetupAns,
					MACCommands: []lorawan.MACCommand{
						{
							CID: lorawan.RejoinParamSetupAns,
							Payload: &lorawan.RejoinParamSetupAnsPayload{
								TimeOK: true,
							},
						},
					},
				},
				ExpectedError: errors.New("expected pending mac-command"),
				ExpectedDeviceSession: storage.DeviceSession{
					RejoinRequestMaxCountN: 1,
					RejoinRequestMaxTimeN:  2,
				},
			},
		}

		for i, test := range tests {
			Convey(fmt.Sprintf("Testing: %s [%d]", test.Name, i), func() {
				ans, err := handleRejoinParamSetupAns(context.Background(), &test.DeviceSession, test.ReceivedMACCommandBlock, test.PendingMACCommandBlock)
				if test.ExpectedError != nil {
					So(err, ShouldNotBeNil)
					So(err.Error(), ShouldEqual, test.ExpectedError.Error())
				} else {
					So(err, ShouldBeNil)
				}
				So(ans, ShouldBeNil)
				So(test.DeviceSession, ShouldResemble, test.ExpectedDeviceSession)
			})
		}
	})
}
