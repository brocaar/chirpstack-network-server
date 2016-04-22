package mqttpubsub

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/brocaar/loraserver/internal/loraserver"
	"github.com/brocaar/loraserver/models"
	"github.com/brocaar/lorawan"
	"github.com/eclipse/paho.mqtt.golang"
	. "github.com/smartystreets/goconvey/convey"
)

func TestBackend(t *testing.T) {
	conf := getConfig()

	Convey("Given a MQTT client and Redis database", t, func() {
		opts := mqtt.NewClientOptions().AddBroker(conf.Server).SetUsername(conf.Username).SetPassword(conf.Password)
		c := mqtt.NewClient(opts)
		token := c.Connect()
		token.Wait()
		So(token.Error(), ShouldBeNil)

		p := loraserver.NewRedisPool(conf.RedisURL)

		Convey("Given a new Backend", func() {
			backend, err := NewBackend(p, conf.Server, conf.Username, conf.Password)
			So(err, ShouldBeNil)
			defer backend.Close()
			time.Sleep(time.Millisecond * 100) // give the backend some time to subscribe to the topic

			Convey("Given the MQTT client is subscribed to node/+/rx", func() {
				rxPacketChan := make(chan models.RXPayload)
				token := c.Subscribe("application/+/node/+/rx", 0, func(c mqtt.Client, msg mqtt.Message) {
					var rxPacket models.RXPayload
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

					rxPacket := models.RXPayload{
						DevEUI: devEUI,
					}
					So(backend.Send(devEUI, appEUI, rxPacket), ShouldBeNil)

					Convey("Then the same packet is consumed by the MQTT client", func() {
						packet := <-rxPacketChan
						So(packet, ShouldResemble, rxPacket)
					})

				})
			})

			Convey("Given a ApplicationTXPayload is published by the MQTT client", func() {
				pl := models.TXPayload{
					Confirmed: false,
					DevEUI:    [8]byte{8, 7, 6, 5, 4, 3, 2, 1},
					FPort:     1,
					Data:      []byte("hello!"),
				}
				b, err := json.Marshal(pl)
				So(err, ShouldBeNil)
				token := c.Publish("application/0102030405060708/node/0807060504030201/tx", 0, false, b)
				token.Wait()
				So(token.Error(), ShouldBeNil)

				Convey("Then the same packet is received by the backend", func() {
					p := <-backend.TXPayloadChan()
					So(p, ShouldResemble, pl)

					Convey("When the topic DevEUI does not match the payload DevEUI", func() {
						token := c.Publish("application/0102030405060708/node/0707060504030201/tx", 0, false, b)
						token.Wait()
						So(token.Error(), ShouldBeNil)

						Convey("Then the packet is discarded", func() {
							var received bool
							select {
							case <-backend.TXPayloadChan():
								received = true
							case <-time.After(time.Millisecond * 100):
								// nothing to do
							}
							So(received, ShouldBeFalse)
						})
					})
				})
			})
		})
	})
}
