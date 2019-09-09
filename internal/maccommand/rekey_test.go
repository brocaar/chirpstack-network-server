package maccommand

import (
	"context"
	"fmt"
	"testing"

	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/lorawan"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRekey(t *testing.T) {
	Convey("Given a set of tests", t, func() {
		tests := []struct {
			Name                    string
			DevMinorVersion         uint8
			ExpectedMACCommandBlock storage.MACCommandBlock
		}{
			{
				Name:            "LoRaWAN 1.1 device",
				DevMinorVersion: 1,
				ExpectedMACCommandBlock: storage.MACCommandBlock{
					CID: lorawan.RekeyConf,
					MACCommands: []lorawan.MACCommand{
						{
							CID: lorawan.RekeyConf,
							Payload: &lorawan.RekeyConfPayload{
								ServLoRaWANVersion: lorawan.Version{
									Minor: 1,
								},
							},
						},
					},
				},
			},
		}

		for i, test := range tests {
			Convey(fmt.Sprintf("Testing: %s [%d]", test.Name, i), func() {
				req := storage.MACCommandBlock{
					CID: lorawan.RekeyInd,
					MACCommands: []lorawan.MACCommand{
						{
							CID: lorawan.RekeyInd,
							Payload: &lorawan.RekeyIndPayload{
								DevLoRaWANVersion: lorawan.Version{
									Minor: test.DevMinorVersion,
								},
							},
						},
					},
				}

				ds := storage.DeviceSession{}

				out, err := handleRekeyInd(context.Background(), &ds, req)
				So(err, ShouldBeNil)
				So(out, ShouldHaveLength, 1)
				So(out[0], ShouldResemble, test.ExpectedMACCommandBlock)
			})
		}
	})
}
