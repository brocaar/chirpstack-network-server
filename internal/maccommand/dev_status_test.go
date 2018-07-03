package maccommand

import (
	"fmt"
	"testing"
	"time"

	"github.com/brocaar/lorawan"

	"github.com/brocaar/loraserver/api/as"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/loraserver/internal/test"
	. "github.com/smartystreets/goconvey/convey"
)

func TestRequestDevStatus(t *testing.T) {
	Convey("When calling RequestDevStatus", t, func() {
		ds := storage.DeviceSession{}
		block := RequestDevStatus(&ds)

		Convey("Then the expected block is returned", func() {
			So(block, ShouldResemble, storage.MACCommandBlock{
				CID: lorawan.DevStatusReq,
				MACCommands: []lorawan.MACCommand{
					{
						CID: lorawan.DevStatusReq,
					},
				},
			})
		})

		Convey("Then the device-session has been updated", func() {
			So(time.Now().Sub(ds.LastDevStatusRequested), ShouldBeLessThan, time.Second)
		})
	})
}

func TestDevStatusAns(t *testing.T) {
	Convey("Given a set of tests", t, func() {
		tests := []struct {
			Name                           string
			DeviceSession                  storage.DeviceSession
			ServiceProfile                 storage.ServiceProfile
			ReceivedMACCommandBlock        storage.MACCommandBlock
			ExpectedSetDeviceStatusRequest as.SetDeviceStatusRequest
		}{
			{
				Name: "report device-status",
				DeviceSession: storage.DeviceSession{
					DevEUI: lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
				},
				ServiceProfile: storage.ServiceProfile{
					ReportDevStatusBattery: true,
					ReportDevStatusMargin:  true,
				},
				ReceivedMACCommandBlock: storage.MACCommandBlock{
					CID: lorawan.DevStatusAns,
					MACCommands: []lorawan.MACCommand{
						{
							CID: lorawan.DevStatusAns,
							Payload: &lorawan.DevStatusAnsPayload{
								Margin:  10,
								Battery: 150,
							},
						},
					},
				},
				ExpectedSetDeviceStatusRequest: as.SetDeviceStatusRequest{
					DevEui:  []byte{1, 2, 3, 4, 5, 6, 7, 8},
					Battery: 150,
					Margin:  10,
				},
			},
			{
				Name: "report device-status battery only",
				DeviceSession: storage.DeviceSession{
					DevEUI: lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
				},
				ServiceProfile: storage.ServiceProfile{
					ReportDevStatusBattery: true,
				},
				ReceivedMACCommandBlock: storage.MACCommandBlock{
					CID: lorawan.DevStatusAns,
					MACCommands: []lorawan.MACCommand{
						{
							CID: lorawan.DevStatusAns,
							Payload: &lorawan.DevStatusAnsPayload{
								Margin:  10,
								Battery: 150,
							},
						},
					},
				},
				ExpectedSetDeviceStatusRequest: as.SetDeviceStatusRequest{
					DevEui:  []byte{1, 2, 3, 4, 5, 6, 7, 8},
					Battery: 150,
				},
			},
			{
				Name: "report device-status margin only",
				DeviceSession: storage.DeviceSession{
					DevEUI: lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
				},
				ServiceProfile: storage.ServiceProfile{
					ReportDevStatusMargin: true,
				},
				ReceivedMACCommandBlock: storage.MACCommandBlock{
					CID: lorawan.DevStatusAns,
					MACCommands: []lorawan.MACCommand{
						{
							CID: lorawan.DevStatusAns,
							Payload: &lorawan.DevStatusAnsPayload{
								Margin:  10,
								Battery: 150,
							},
						},
					},
				},
				ExpectedSetDeviceStatusRequest: as.SetDeviceStatusRequest{
					DevEui: []byte{1, 2, 3, 4, 5, 6, 7, 8},
					Margin: 10,
				},
			},
		}

		for i, t := range tests {
			Convey(fmt.Sprintf("Testing: %s [%d]", t.Name, i), func() {
				asClient := test.NewApplicationClient()
				resp, err := handleDevStatusAns(&t.DeviceSession, t.ServiceProfile, asClient, t.ReceivedMACCommandBlock)
				So(resp, ShouldHaveLength, 0)
				So(err, ShouldBeNil)

				So(<-asClient.SetDeviceStatusChan, ShouldResemble, t.ExpectedSetDeviceStatusRequest)
			})
		}
	})
}
