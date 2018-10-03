package maccommand

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/brocaar/loraserver/api/as"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/loraserver/internal/test"
	"github.com/brocaar/lorawan"
)

type DevStatusTestSuite struct {
	suite.Suite
}

func (ts *DevStatusTestSuite) TestRequestDevStatus() {
	assert := require.New(ts.T())

	ds := storage.DeviceSession{}
	block := RequestDevStatus(&ds)

	assert.Equal(storage.MACCommandBlock{
		CID: lorawan.DevStatusReq,
		MACCommands: []lorawan.MACCommand{
			{
				CID: lorawan.DevStatusReq,
			},
		},
	}, block)

	assert.InDelta(ds.LastDevStatusRequested.UnixNano(), time.Now().UnixNano(), float64(time.Second))
}

func (ts *DevStatusTestSuite) TestDevStatusAns() {
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
				DevEui:       []byte{1, 2, 3, 4, 5, 6, 7, 8},
				Battery:      150,
				Margin:       10,
				BatteryLevel: float32(150) / float32(254) * float32(100),
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
				DevEui:       []byte{1, 2, 3, 4, 5, 6, 7, 8},
				Battery:      150,
				BatteryLevel: float32(150) / float32(254) * float32(100),
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
				DevEui:                  []byte{1, 2, 3, 4, 5, 6, 7, 8},
				Margin:                  10,
				BatteryLevelUnavailable: true,
			},
		},
	}

	for _, tst := range tests {
		ts.T().Run(tst.Name, func(t *testing.T) {
			assert := require.New(t)
			asClient := test.NewApplicationClient()
			resp, err := handleDevStatusAns(&tst.DeviceSession, tst.ServiceProfile, asClient, tst.ReceivedMACCommandBlock)
			assert.NoError(err)
			assert.Len(resp, 0)

			assert.Equal(tst.ExpectedSetDeviceStatusRequest, <-asClient.SetDeviceStatusChan)
		})
	}
}

func TestDevStatus(t *testing.T) {
	suite.Run(t, new(DevStatusTestSuite))
}
