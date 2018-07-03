package testsuite

import (
	"context"
	"fmt"
	"testing"

	"github.com/brocaar/loraserver/api/as"
	commonPB "github.com/brocaar/loraserver/api/common"
	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/api/ns"
	"github.com/brocaar/loraserver/internal/api"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/loraserver/internal/test"
	"github.com/brocaar/loraserver/internal/uplink"
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

	ExpectedApplicationHandleProprietaryUp *as.HandleProprietaryUplinkRequest
}

func TestSendProprietaryPayloadScenarios(t *testing.T) {
	conf := test.GetConfig()
	db, err := common.OpenDatabase(conf.PostgresDSN)
	if err != nil {
		t.Fatal(err)
	}
	config.C.PostgreSQL.DB = db

	Convey("Given a clean state", t, func() {
		test.MustResetDB(config.C.PostgreSQL.DB)

		config.C.NetworkServer.Gateway.Backend.Backend = test.NewGatewayBackend()
		api := api.NewNetworkServerAPI()

		Convey("Given a set of send proprietary payload tests", func() {
			trueBool := true
			dr5, err := config.C.NetworkServer.Band.Band.GetDataRate(5)
			So(err, ShouldBeNil)

			tests := []sendProprietaryPayloadTestCase{
				{
					Name: "send proprietary payload",
					SendProprietaryPayloadRequest: ns.SendProprietaryPayloadRequest{
						MacPayload:            []byte{1, 2, 3, 4},
						Mic:                   []byte{5, 6, 7, 8},
						GatewayMacs:           [][]byte{{8, 7, 6, 5, 4, 3, 2, 1}},
						PolarizationInversion: true,
						Frequency:             868100000,
						Dr:                    5,
					},
					ExpectedTXInfo: gw.TXInfo{
						MAC:         lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1},
						Immediately: true,
						Frequency:   868100000,
						Power:       14,
						DataRate:    dr5,
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
						So(config.C.NetworkServer.Gateway.Backend.Backend.(*test.GatewayBackend).TXPacketChan, ShouldHaveLength, 1)
						txPacket := <-config.C.NetworkServer.Gateway.Backend.Backend.(*test.GatewayBackend).TXPacketChan
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
	config.C.PostgreSQL.DB = db
	config.C.Redis.Pool = common.NewRedisPool(conf.RedisURL)

	Convey("Given a clean state with a gateway and routing profile", t, func() {
		test.MustResetDB(config.C.PostgreSQL.DB)
		test.MustFlushRedis(config.C.Redis.Pool)

		asClient := test.NewApplicationClient()
		config.C.ApplicationServer.Pool = test.NewApplicationServerPool(asClient)
		config.C.NetworkServer.Gateway.Backend.Backend = test.NewGatewayBackend()

		// the routing profile is needed as the ns will send the proprietary
		// frame to all application-servers.
		rp := storage.RoutingProfile{}
		So(storage.CreateRoutingProfile(config.C.PostgreSQL.DB, &rp), ShouldBeNil)

		g := storage.Gateway{
			MAC: lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
			Location: storage.GPSPoint{
				Latitude:  1.1234,
				Longitude: 2.345,
			},
			Altitude: 10,
		}
		So(storage.CreateGateway(config.C.PostgreSQL.DB, &g), ShouldBeNil)

		Convey("Given a set of testcases", func() {
			dr0, err := config.C.NetworkServer.Band.Band.GetDataRate(0)
			So(err, ShouldBeNil)

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
						DataRate:  dr0,
					},
					ExpectedApplicationHandleProprietaryUp: &as.HandleProprietaryUplinkRequest{
						MacPayload: []byte{1, 2, 3, 4},
						Mic:        []byte{5, 6, 7, 8},
						TxInfo: &gw.UplinkTXInfo{
							Frequency:  868100000,
							Modulation: commonPB.Modulation_LORA,
							ModulationInfo: &gw.UplinkTXInfo_LoraModulationInfo{
								LoraModulationInfo: &gw.LoRaModulationInfo{
									Bandwidth:       125,
									SpreadingFactor: 12,
									CodeRate:        "4/5",
								},
							},
						},
						RxInfo: []*gw.UplinkRXInfo{
							{
								GatewayId: []byte{1, 2, 3, 4, 5, 6, 7, 8},
								Rssi:      -10,
								LoraSnr:   5,
								Location: &gw.Location{
									Latitude:  1.1234,
									Longitude: 2.345,
									Altitude:  10,
								},
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
							req := <-asClient.HandleProprietaryUpChan
							So(&req, ShouldResemble, t.ExpectedApplicationHandleProprietaryUp)
						})
					}
				})
			}
		})
	})

}
