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
	devEUI := lorawan.EUI64{1, 1, 1, 1, 1, 1, 1, 1}
	appEUI := lorawan.EUI64{2, 2, 2, 2, 2, 2, 2, 2}

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
			time.Sleep(time.Millisecond * 100) // give the backend some time to subscribe

			Convey("Given the MQTT client is subscribed to application/+/node/+/rxinfo", func() {
				rxInfoPLChan := make(chan models.RXInfoPayload)
				token := c.Subscribe("application/+/node/+/rxinfo", 0, func(c mqtt.Client, msg mqtt.Message) {
					var rxInfoPayload models.RXInfoPayload
					if err := json.Unmarshal(msg.Payload(), &rxInfoPayload); err != nil {
						t.Fatal(err)
					}
					rxInfoPLChan <- rxInfoPayload
				})
				token.Wait()
				So(token.Error(), ShouldBeNil)

				Convey("When sending a RXInfoPayload from the backend", func() {
					pl := models.RXInfoPayload{
						RXInfo: []models.RXInfo{
							{Time: time.Now().UTC()},
						},
					}
					So(backend.SendRXInfoPayload(appEUI, devEUI, pl), ShouldBeNil)

					Convey("Then the same packet was received by the MQTT client", func() {
						pl2 := <-rxInfoPLChan
						So(pl2, ShouldResemble, pl)
					})
				})
			})

			Convey("Given the MQTT client is subscribed to application/+/node/+/mac/rx", func() {
				macChan := make(chan models.MACPayload)
				token := c.Subscribe("application/+/node/+/mac/rx", 0, func(c mqtt.Client, msg mqtt.Message) {
					var mac models.MACPayload
					if err := json.Unmarshal(msg.Payload(), &mac); err != nil {
						panic(err)
					}
					macChan <- mac
				})
				token.Wait()
				So(token.Error(), ShouldBeNil)

				Convey("When sending a MACCommand from the backend", func() {
					mac := models.MACPayload{
						MACCommand: []byte{1, 2, 3},
					}
					So(backend.SendMACPayload(appEUI, devEUI, mac), ShouldBeNil)

					Convey("Then the same MACCommand was received by the MQTT client", func() {
						mac2 := <-macChan
						So(mac2, ShouldResemble, mac)
					})
				})
			})

			Convey("Given the MQTT client publishes a MAC command to application/01010101010101/node/0202020202020202/mac/tx", func() {
				topic := "application/01010101010101/node/0202020202020202/mac/tx"
				mac := models.MACPayload{
					DevEUI:     [8]byte{2, 2, 2, 2, 2, 2, 2, 2},
					Reference:  "abc123",
					MACCommand: []byte{1, 2, 3},
				}
				b, err := json.Marshal(mac)
				So(err, ShouldBeNil)
				token := c.Publish(topic, 0, false, b)
				token.Wait()
				So(token.Error(), ShouldBeNil)

				Convey("The same MAC command is received by the backend", func() {
					p := <-backend.TXMACPayloadChan()
					So(p, ShouldResemble, mac)

					Convey("When the topic DevEUI does noet match the DevEUI in the message", func() {
						token := c.Publish("application/01010101010101/node/0303030303030303/mac/tx", 0, false, b)
						token.Wait()
						So(token.Error(), ShouldBeNil)

						Convey("Then the command is discarded", func() {
							var received bool
							select {
							case <-backend.TXMACPayloadChan():
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
