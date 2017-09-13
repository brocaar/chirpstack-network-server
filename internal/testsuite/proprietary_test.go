package testsuite

import (
	"context"
	"fmt"
	"testing"

	"github.com/brocaar/loraserver/internal/gateway"
	"github.com/brocaar/loraserver/internal/uplink"

	"github.com/brocaar/loraserver/api/as"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/api/ns"
	"github.com/brocaar/loraserver/internal/api"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/test"
	"github.com/brocaar/lorawan"
	. "github.com/smartystreets/goconvey/convey"
)

type sendProprietaryPayloadTestCase struct {
	Name                          string
	SendProprietaryPayloadRequest ns.SendProprietaryPayloadRequest
	ExpectedTXInfo                gw.TXInfo
	ExpectedPHYPayload            lorawan.PHYPayload
}

type uplinkProprietaryPHYPayloadTestCase struct {
	Name       string
	PHYPayload lorawan.PHYPayload
	RXInfo     gw.RXInfo

	ExpectedApplicationHandleProprietaryUp *as.HandleProprietaryUpRequest
}

func TestSendProprietaryPayloadScenarios(t *testing.T) {
	conf := test.GetConfig()
	db, err := common.OpenDatabase(conf.PostgresDSN)
	if err != nil {
		t.Fatal(err)
	}
	common.DB = db

	Convey("Given a clean state", t, func() {
		test.MustResetDB(common.DB)

		common.Gateway = test.NewGatewayBackend()
		api := api.NewNetworkServerAPI()

		Convey("Given a set of send proprietary payload tests", func() {
			trueBool := true

			tests := []sendProprietaryPayloadTestCase{
				{
					Name: "send proprietary payload",
					SendProprietaryPayloadRequest: ns.SendProprietaryPayloadRequest{
						MacPayload:  []byte{1, 2, 3, 4},
						Mic:         []byte{5, 6, 7, 8},
						GatewayMACs: [][]byte{{8, 7, 6, 5, 4, 3, 2, 1}},
						IPol:        true,
						Frequency:   868100000,
						Dr:          5,
					},
					ExpectedTXInfo: gw.TXInfo{
						MAC:         lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1},
						Immediately: true,
						Frequency:   868100000,
						Power:       14,
						DataRate:    common.Band.DataRates[5],
						CodeRate:    "4/5",
						IPol:        &trueBool,
					},
					ExpectedPHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							Major: lorawan.LoRaWANR1,
							MType: lorawan.Proprietary,
						},
						MACPayload: &lorawan.DataPayload{Bytes: []byte{1, 2, 3, 4}},
						MIC:        lorawan.MIC{5, 6, 7, 8},
					},
				},
			}

			for i, t := range tests {
				Convey(fmt.Sprintf("When testing: %s [%d]", t.Name, i), func() {
					_, err := api.SendProprietaryPayload(context.Background(), &t.SendProprietaryPayloadRequest)
					So(err, ShouldBeNil)

					Convey("Then the expected frame was sent", func() {
						So(common.Gateway.(*test.GatewayBackend).TXPacketChan, ShouldHaveLength, 1)
						txPacket := <-common.Gateway.(*test.GatewayBackend).TXPacketChan
						So(txPacket.TXInfo, ShouldResemble, t.ExpectedTXInfo)
						So(txPacket.PHYPayload, ShouldResemble, t.ExpectedPHYPayload)
					})
				})
			}
		})
	})
}

func TestUplinkProprietaryPHYPayload(t *testing.T) {
	conf := test.GetConfig()
	db, err := common.OpenDatabase(conf.PostgresDSN)
	if err != nil {
		t.Fatal(err)
	}
	common.DB = db
	common.RedisPool = common.NewRedisPool(conf.RedisURL)

	Convey("Given a clean state and a gateway", t, func() {
		test.MustResetDB(common.DB)
		test.MustFlushRedis(common.RedisPool)

		common.Gateway = test.NewGatewayBackend()
		common.Application = test.NewApplicationClient()

		g := gateway.Gateway{
			MAC:         lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
			Name:        "test-gw",
			Description: "test gateway",
			Location: gateway.GPSPoint{
				Latitude:  1.1234,
				Longitude: 2.345,
			},
			Altitude: 10,
		}
		So(gateway.CreateGateway(common.DB, &g), ShouldBeNil)

		Convey("Given a set of testcases", func() {
			tests := []uplinkProprietaryPHYPayloadTestCase{
				{
					Name: "Uplink proprietary payload",
					PHYPayload: lorawan.PHYPayload{
						MHDR: lorawan.MHDR{
							Major: lorawan.LoRaWANR1,
							MType: lorawan.Proprietary,
						},
						MACPayload: &lorawan.DataPayload{Bytes: []byte{1, 2, 3, 4}},
						MIC:        lorawan.MIC{5, 6, 7, 8},
					},
					RXInfo: gw.RXInfo{
						MAC:       lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
						Timestamp: 12345,
						Frequency: 868100000,
						CodeRate:  "4/5",
						RSSI:      -10,
						LoRaSNR:   5,
						DataRate:  common.Band.DataRates[0],
					},
					ExpectedApplicationHandleProprietaryUp: &as.HandleProprietaryUpRequest{
						MacPayload: []byte{1, 2, 3, 4},
						Mic:        []byte{5, 6, 7, 8},
						TxInfo: &as.TXInfo{
							Frequency: 868100000,
							DataRate: &as.DataRate{
								Modulation:   "LORA",
								BandWidth:    125,
								SpreadFactor: 12,
							},
							CodeRate: "4/5",
						},
						RxInfo: []*as.RXInfo{
							{
								Mac:       []byte{1, 2, 3, 4, 5, 6, 7, 8},
								Rssi:      -10,
								LoRaSNR:   5,
								Time:      "0001-01-01T00:00:00Z",
								Name:      "test-gw",
								Latitude:  1.1234,
								Longitude: 2.345,
								Altitude:  10,
							},
						},
					},
				},
			}

			for i, t := range tests {
				Convey(fmt.Sprintf("Testing: %s [%d]", t.Name, i), func() {
					rxPacket := gw.RXPacket{
						RXInfo:     t.RXInfo,
						PHYPayload: t.PHYPayload,
					}
					So(uplink.HandleRXPacket(rxPacket), ShouldBeNil)

					if t.ExpectedApplicationHandleProprietaryUp != nil {
						Convey("Then HandleProprietaryUp was called with the expected data", func() {
							So(common.Application.(*test.ApplicationClient).HandleProprietaryUpChan, ShouldHaveLength, 1)
							req := <-common.Application.(*test.ApplicationClient).HandleProprietaryUpChan
							So(t.ExpectedApplicationHandleProprietaryUp, ShouldResemble, &req)
						})
					}
				})
			}
		})
	})

}
