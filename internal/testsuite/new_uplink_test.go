package testsuite

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/brocaar/loraserver/api/common"
	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/loraserver/internal/uplink"
	"github.com/brocaar/lorawan"
)

type UplinkTest struct {
	UplinkIntegrationTestSuite
}

func (t *UplinkTest) SetupTest() {
	t.UplinkIntegrationTestSuite.SetupTest()

	t.CreateGateway(storage.Gateway{MAC: lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1}})
	t.CreateDevice(storage.Device{DevEUI: lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}})
	t.CreateDeviceSession(storage.DeviceSession{})
}

// this tests that on uplink data, the device gateway rx-info set has been
// stored in redis
func (t *UplinkTest) TestDeviceGatewayRXInfoSetHasBeenStored() {
	txInfo := gw.UplinkTXInfo{
		Frequency:  868300000,
		Modulation: common.Modulation_LORA,
		ModulationInfo: &gw.UplinkTXInfo_LoraModulationInfo{
			LoraModulationInfo: &gw.LoRaModulationInfo{
				SpreadingFactor: 10,
				Bandwidth:       125,
			},
		},
	}
	rxInfo := gw.UplinkRXInfo{
		GatewayId: t.Gateway.MAC[:],
		Rssi:      -60,
		LoraSnr:   5.5,
	}

	t.Require().Nil(uplink.HandleRXPacket(t.GetUplinkFrameForFRMPayload(rxInfo, txInfo, lorawan.UnconfirmedDataUp, 10, []byte{1, 2, 3, 4})))

	rxInfoSet, err := storage.GetDeviceGatewayRXInfoSet(config.C.Redis.Pool, t.Device.DevEUI)
	t.Require().Nil(err)
	t.Equal(storage.DeviceGatewayRXInfoSet{
		DevEUI: t.Device.DevEUI,
		DR:     2,
		Items: []storage.DeviceGatewayRXInfo{
			{
				GatewayID: t.Gateway.MAC,
				RSSI:      -60,
				LoRaSNR:   5.5,
			},
		},
	}, rxInfoSet)

}

func TestUplink(t *testing.T) {
	suite.Run(t, new(UplinkTest))
}
