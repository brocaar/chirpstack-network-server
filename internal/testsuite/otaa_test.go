package testsuite

import (
	"errors"
	"fmt"
	"testing"

	"github.com/brocaar/loraserver/api/as"
	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/test"
	"github.com/brocaar/loraserver/internal/uplink"
	"github.com/brocaar/lorawan"
	. "github.com/smartystreets/goconvey/convey"
)

type otaaTestCase struct {
	Name                           string                 // name of the test
	RXInfo                         gw.RXInfo              // rx-info of the "received" packet
	PHYPayload                     lorawan.PHYPayload     // received PHYPayload
	ApplicationJoinRequestResponse as.JoinRequestResponse // application-client join-request response
	ApplicationJoinRequestError    error                  // application-client join-request error
	AppKey                         lorawan.AES128Key      // app-key (used to decrypt the expected PHYPayload)

	ExpectedError                         error                 // expected error
	ExpectedApplicationJoinRequestRequest as.JoinRequestRequest // expected join-request request
	ExpectedTXInfo                        gw.TXInfo             // expected tx-info
	ExpectedPHYPayload                    lorawan.PHYPayload    // expected (plaintext) PHYPayload
}

func TestOTAAScenarios(t *testing.T) {
	conf := test.GetTestConfig()

	Convey("Given a clean state", t, func() {
		p := common.NewRedisPool(conf.RedisURL)
		test.MustFlushRedis(p)

		ctx := common.Context{
			NetID:       [3]byte{3, 2, 1},
			RedisPool:   p,
			Gateway:     test.NewTestGatewayBackend(),
			Application: test.NewTestApplicationClient(),
		}

		appKey := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}

		rxInfo := gw.RXInfo{
			Frequency: common.Band.UplinkChannels[0].Frequency,
			DataRate:  common.Band.DataRates[common.Band.UplinkChannels[0].DataRates[0]],
		}

		jrPayload := lorawan.PHYPayload{
			MHDR: lorawan.MHDR{
				MType: lorawan.JoinRequest,
				Major: lorawan.LoRaWANR1,
			},
			MACPayload: &lorawan.JoinRequestPayload{
				AppEUI:   [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
				DevEUI:   [8]byte{2, 2, 3, 4, 5, 6, 7, 8},
				DevNonce: [2]byte{1, 2},
			},
		}
		So(jrPayload.SetMIC(appKey), ShouldBeNil)
		jrBytes, err := jrPayload.MarshalBinary()
		So(err, ShouldBeNil)

		jaPayload := lorawan.JoinAcceptPayload{
			AppNonce: [3]byte{3, 2, 1},
			NetID:    ctx.NetID,
			DLSettings: lorawan.DLSettings{
				RX2DataRate: 2,
				RX1DROffset: 1,
			},
			DevAddr: [4]byte{1, 2, 3, 4},
			RXDelay: 3,
			CFList:  &lorawan.CFList{100, 200, 300, 400, 500},
		}
		jaPHY := lorawan.PHYPayload{
			MHDR: lorawan.MHDR{
				MType: lorawan.JoinAccept,
				Major: lorawan.LoRaWANR1,
			},
			MACPayload: &jaPayload,
		}
		So(jaPHY.SetMIC(appKey), ShouldBeNil)
		So(jaPHY.EncryptJoinAcceptPayload(appKey), ShouldBeNil)
		jaBytes, err := jaPHY.MarshalBinary()
		So(err, ShouldBeNil)
		So(jaPHY.DecryptJoinAcceptPayload(appKey), ShouldBeNil)

		Convey("Given a set of test-scenarios", func() {
			tests := []otaaTestCase{
				{
					Name:                        "application-client returns an error",
					RXInfo:                      rxInfo,
					PHYPayload:                  jrPayload,
					ApplicationJoinRequestError: errors.New("BOOM!"),
					ExpectedError:               errors.New("application server join-request error: BOOM!"),
				},
				{
					Name:       "join-accept using rx1",
					RXInfo:     rxInfo,
					PHYPayload: jrPayload,
					ApplicationJoinRequestResponse: as.JoinRequestResponse{
						PhyPayload:  jaBytes,
						NwkSKey:     []byte{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
						RxDelay:     uint32(jaPayload.RXDelay),
						Rx1DROffset: uint32(jaPayload.DLSettings.RX1DROffset),
						CFList:      jaPayload.CFList[:],
						RxWindow:    as.RXWindow_RX1,
						Rx2DR:       uint32(jaPayload.DLSettings.RX2DataRate),
					},
					AppKey: appKey,

					ExpectedApplicationJoinRequestRequest: as.JoinRequestRequest{
						PhyPayload: jrBytes,
						NetID:      []byte{3, 2, 1},
						DevAddr:    []byte{0, 0, 0, 0},
					},
					ExpectedTXInfo: gw.TXInfo{
						MAC:       rxInfo.MAC,
						Timestamp: rxInfo.Timestamp + 5000000,
						Frequency: rxInfo.Frequency,
						Power:     14,
						DataRate:  rxInfo.DataRate,
						CodeRate:  rxInfo.CodeRate,
					},
					ExpectedPHYPayload: jaPHY,
				},
				{
					Name:       "join-accept using rx2",
					RXInfo:     rxInfo,
					PHYPayload: jrPayload,
					ApplicationJoinRequestResponse: as.JoinRequestResponse{
						PhyPayload:  jaBytes,
						NwkSKey:     []byte{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
						RxDelay:     uint32(jaPayload.RXDelay),
						Rx1DROffset: uint32(jaPayload.DLSettings.RX1DROffset),
						CFList:      jaPayload.CFList[:],
						RxWindow:    as.RXWindow_RX2,
						Rx2DR:       uint32(jaPayload.DLSettings.RX2DataRate),
					},
					AppKey: appKey,

					ExpectedApplicationJoinRequestRequest: as.JoinRequestRequest{
						PhyPayload: jrBytes,
						NetID:      []byte{3, 2, 1},
						DevAddr:    []byte{0, 0, 0, 0},
					},
					ExpectedTXInfo: gw.TXInfo{
						MAC:       rxInfo.MAC,
						Timestamp: rxInfo.Timestamp + 6000000,
						Frequency: common.Band.RX2Frequency,
						Power:     14,
						DataRate:  common.Band.DataRates[common.Band.RX2DataRate],
						CodeRate:  rxInfo.CodeRate,
					},
					ExpectedPHYPayload: jaPHY,
				},
			}

			runOTAATests(ctx, tests)
		})
	})
}

func runOTAATests(ctx common.Context, tests []otaaTestCase) {
	for i, t := range tests {
		Convey(fmt.Sprintf("When testing: %s [%d]", t.Name, i), func() {
			// set mocks
			ctx.Application.(*test.TestApplicationClient).Err = t.ApplicationJoinRequestError
			ctx.Application.(*test.TestApplicationClient).JoinRequestResponse = t.ApplicationJoinRequestResponse

			So(uplink.HandleRXPacket(ctx, gw.RXPacket{
				RXInfo:     t.RXInfo,
				PHYPayload: t.PHYPayload,
			}), ShouldResemble, t.ExpectedError)

			if t.ExpectedError != nil {
				return
			}

			Convey("Then the expected join-request request was made to the application server", func() {
				So(ctx.Application.(*test.TestApplicationClient).JoinRequestChan, ShouldHaveLength, 1)
				req := <-ctx.Application.(*test.TestApplicationClient).JoinRequestChan

				So(req.DevAddr, ShouldHaveLength, 4)
				So(req.DevAddr, ShouldNotResemble, []byte{0, 0, 0, 0})
				req.DevAddr = []byte{0, 0, 0, 0} // as this is random, we can't compare it unless it is set to a known value

				So(req, ShouldResemble, t.ExpectedApplicationJoinRequestRequest)
			})

			Convey("Then the expected txinfo is used", func() {
				So(ctx.Gateway.(*test.TestGatewayBackend).TXPacketChan, ShouldHaveLength, 1)
				txPacket := <-ctx.Gateway.(*test.TestGatewayBackend).TXPacketChan

				So(txPacket.TXInfo, ShouldResemble, t.ExpectedTXInfo)
			})

			Convey("Then the expected PHYPayload was sent", func() {
				So(ctx.Gateway.(*test.TestGatewayBackend).TXPacketChan, ShouldHaveLength, 1)
				txPacket := <-ctx.Gateway.(*test.TestGatewayBackend).TXPacketChan

				So(txPacket.PHYPayload.DecryptJoinAcceptPayload(t.AppKey), ShouldBeNil)
				So(txPacket.PHYPayload, ShouldResemble, t.ExpectedPHYPayload)
			})
		})
	}
}
