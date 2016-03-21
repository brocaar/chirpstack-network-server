package mqttpubsub

import (
	"encoding/json"
	"testing"

	"github.com/brocaar/loraserver"
	"github.com/brocaar/lorawan"
	"github.com/eclipse/paho.mqtt.golang"
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

		Convey("Given a new Backend", func() {
			backend, err := NewBackend(conf.Server, conf.Username, conf.Password)
			So(err, ShouldBeNil)
			defer backend.Close()

			Convey("Given the MQTT client is subscribed to node/+/rx", func() {
				rxPacketChan := make(chan loraserver.ApplicationRXPayload)
				token := c.Subscribe("application/+/node/+/rx", 0, func(c *mqtt.Client, msg mqtt.Message) {
					var rxPacket loraserver.ApplicationRXPayload
					if err := json.Unmarshal(msg.Payload(), &rxPacket); err != nil {
						t.Fatal(err)
					}
					rxPacketChan <- rxPacket
				})
				token.Wait()
				So(token.Error(), ShouldBeNil)

				Convey("When sending a ApplicationRXPacket (from the backend)", func() {
					devEUI := lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 1}
					appEUI := lorawan.EUI64{2, 2, 2, 2, 2, 2, 2, 2}

					rxPacket := loraserver.ApplicationRXPayload{
						MType:  lorawan.ConfirmedDataUp,
						DevEUI: devEUI,
					}
					So(backend.Send(devEUI, appEUI, rxPacket), ShouldBeNil)

					Convey("Then the same packet is consumed by the MQTT client", func() {
						packet := <-rxPacketChan
						So(packet, ShouldResemble, rxPacket)
					})

				})
			})
		})
	})
}
