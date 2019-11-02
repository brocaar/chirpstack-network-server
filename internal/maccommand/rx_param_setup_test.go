package maccommand

import (
	"context"
	"fmt"
	"testing"

	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/brocaar/lorawan"
	. "github.com/smartystreets/goconvey/convey"
)

func TestRequestRXParamSetup(t *testing.T) {
	Convey("When calling RequestRXParamSetup", t, func() {
		block := RequestRXParamSetup(2, 868700000, 5)

		Convey("Then the expected block is returned", func() {
			So(block, ShouldResemble, storage.MACCommandBlock{
				CID: lorawan.RXParamSetupReq,
				MACCommands: []lorawan.MACCommand{
					{
						CID: lorawan.RXParamSetupReq,
						Payload: &lorawan.RXParamSetupReqPayload{
							Frequency: 868700000,
							DLSettings: lorawan.DLSettings{
								RX2DataRate: 5,
								RX1DROffset: 2,
							},
						},
					},
				},
			})
		})
	})
}

func TestHandleRXParamSetupAns(t *testing.T) {
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
				Name: "rx param setup ack",
				DeviceSession: storage.DeviceSession{
					RX2Frequency: 868100000,
					RX2DR:        0,
					RX1DROffset:  1,
				},
				ReceivedMACCommandBlock: storage.MACCommandBlock{
					CID: lorawan.RXParamSetupAns,
					MACCommands: storage.MACCommands{
						{
							CID: lorawan.RXParamSetupAns,
							Payload: &lorawan.RXParamSetupAnsPayload{
								ChannelACK:     true,
								RX2DataRateACK: true,
								RX1DROffsetACK: true,
							},
						},
					},
				},
				PendingMACCommandBlock: &storage.MACCommandBlock{
					CID: lorawan.RXParamSetupReq,
					MACCommands: storage.MACCommands{
						{
							CID: lorawan.RXParamSetupReq,
							Payload: &lorawan.RXParamSetupReqPayload{
								Frequency: 868700000,
								DLSettings: lorawan.DLSettings{
									RX2DataRate: 5,
									RX1DROffset: 2,
								},
							},
						},
					},
				},
				ExpectedDeviceSession: storage.DeviceSession{
					RX2Frequency: 868700000,
					RX2DR:        5,
					RX1DROffset:  2,
				},
			},
			{
				Name: "rx param setup nack",
				DeviceSession: storage.DeviceSession{
					RX2Frequency: 868100000,
					RX2DR:        0,
					RX1DROffset:  1,
				},
				ReceivedMACCommandBlock: storage.MACCommandBlock{
					CID: lorawan.RXParamSetupAns,
					MACCommands: storage.MACCommands{
						{
							CID: lorawan.RXParamSetupAns,
							Payload: &lorawan.RXParamSetupAnsPayload{
								ChannelACK:     true,
								RX2DataRateACK: false,
								RX1DROffsetACK: true,
							},
						},
					},
				},
				PendingMACCommandBlock: &storage.MACCommandBlock{
					CID: lorawan.RXParamSetupReq,
					MACCommands: storage.MACCommands{
						{
							CID: lorawan.RXParamSetupReq,
							Payload: &lorawan.RXParamSetupReqPayload{
								Frequency: 868700000,
								DLSettings: lorawan.DLSettings{
									RX2DataRate: 5,
									RX1DROffset: 2,
								},
							},
						},
					},
				},
				ExpectedDeviceSession: storage.DeviceSession{
					RX2Frequency: 868100000,
					RX2DR:        0,
					RX1DROffset:  1,
				},
			},
		}

		for i, t := range tests {
			Convey(fmt.Sprintf("Testing: %s [%d]", t.Name, i), func() {
				ans, err := handleRXParamSetupAns(context.Background(), &t.DeviceSession, t.ReceivedMACCommandBlock, t.PendingMACCommandBlock)
				So(err, ShouldResemble, t.ExpectedError)
				So(ans, ShouldBeNil)
				So(t.DeviceSession, ShouldResemble, t.ExpectedDeviceSession)
			})
		}
	})
}
