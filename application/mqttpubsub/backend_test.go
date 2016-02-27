package mqttpubsub

import (
	"bytes"
	"encoding/gob"
	"testing"

	"git.eclipse.org/gitroot/paho/org.eclipse.paho.mqtt.golang.git"

	"github.com/brocaar/loraserver"
	"github.com/brocaar/lorawan"
	. "github.com/smartystreets/goconvey/convey"
)

func TestBackend(t *testing.T) {
	conf := getConfig()

	Convey("Given a MQTT client", t, func() {
		opts := mqtt.NewClientOptions().AddBroker(conf.Server).SetUsername(conf.Username).SetPassword(conf.Password)
		c := mqtt.NewClient(opts)
		token := c.Connect()
		token.Wait()
		So(token.Error(), ShouldBeNil)
		defer c.Disconnect(0)

		Convey("Given a new Backend", func() {
			backend, err := NewBackend(conf.Server, conf.Username, conf.Password)
			So(err, ShouldBeNil)
			defer backend.Close()

			Convey("Given the MQTT client is subscribed to node/+/rx", func() {
				appRXPayloadChan := make(chan ApplicationRXPayload)
				token := c.Subscribe("node/+/rx", 0, func(c *mqtt.Client, msg mqtt.Message) {
					var appRXPayload ApplicationRXPayload
					dec := gob.NewDecoder(bytes.NewReader(msg.Payload()))
					if err := dec.Decode(&appRXPayload); err != nil {
						t.Fatal(err)
					}
					appRXPayloadChan <- appRXPayload
				})
				token.Wait()
				So(token.Error(), ShouldBeNil)

				Convey("When sending a RXPacket (from the backend)", func() {
					devEUI := lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 1}
					appEUI := lorawan.EUI64{2, 2, 2, 2, 2, 2, 2, 2}

					phy := lorawan.NewPHYPayload(true)
					phy.MHDR = lorawan.MHDR{
						MType: lorawan.ConfirmedDataUp,
						Major: lorawan.LoRaWANR1,
					}

					macPL := lorawan.NewMACPayload(true)
					macPL.FPort = 10
					macPL.FRMPayload = []lorawan.Payload{
						&lorawan.DataPayload{
							Bytes: []byte("Hello!"),
						},
					}
					phy.MACPayload = macPL

					rxPackets := loraserver.RXPackets{
						{
							RXInfo: loraserver.RXInfo{
								MAC: [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
							},
							PHYPayload: phy,
						},
					}

					So(backend.Send(devEUI, appEUI, rxPackets), ShouldBeNil)

					Convey("Then the same packet is consumed by the MQTT client", func() {
						packet := <-appRXPayloadChan
						So(packet, ShouldResemble, ApplicationRXPayload{
							MType:        lorawan.ConfirmedDataUp,
							DevEUI:       devEUI,
							GatewayCount: 1,
							ACK:          false,
							FPort:        10,
							Data:         []byte("Hello!"),
						})
					})
				})
			})
		})
	})
}
