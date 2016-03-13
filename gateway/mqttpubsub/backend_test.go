package mqttpubsub

import (
	"bytes"
	"encoding/gob"
	"testing"

	"git.eclipse.org/gitroot/paho/org.eclipse.paho.mqtt.golang.git"

	"github.com/brocaar/loraserver"
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
		defer c.Disconnect(250)

		Convey("Given a new Backend", func() {
			backend, err := NewBackend(conf.Server, conf.Username, conf.Password)
			So(err, ShouldBeNil)
			defer backend.Close()

			Convey("Given the MQTT client is subscribed to gateway/+/tx", func() {
				txPacketChan := make(chan loraserver.TXPacket)
				token := c.Subscribe("gateway/+/tx", 0, func(c *mqtt.Client, msg mqtt.Message) {
					var txPacket loraserver.TXPacket
					dec := gob.NewDecoder(bytes.NewReader(msg.Payload()))
					if err := dec.Decode(&txPacket); err != nil {
						t.Fatal(err)
					}
					txPacketChan <- txPacket
				})
				token.Wait()
				So(token.Error(), ShouldBeNil)

				Convey("When sending a TXPacket (from the Backend)", func() {
					txPacket := loraserver.TXPacket{
						TXInfo: loraserver.TXInfo{
							MAC: [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
						},
					}
					So(backend.Send(txPacket), ShouldBeNil)

					Convey("Then the same packet is consumed by the MQTT client", func() {
						packet := <-txPacketChan
						So(packet, ShouldResemble, txPacket)
					})
				})

				Convey("When sending a RXPacket (from the MQTT client)", func() {
					rxPacket := loraserver.RXPacket{
						RXInfo: loraserver.RXInfo{
							MAC: [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
						},
					}
					var buf bytes.Buffer
					enc := gob.NewEncoder(&buf)
					So(enc.Encode(rxPacket), ShouldBeNil)
					token := c.Publish("gateway/0102030405060708/rx", 0, false, buf.Bytes())
					token.Wait()
					So(token.Error(), ShouldBeNil)

					Convey("Then the same packet is consumed by the backend", func() {
						packet := <-backend.RXPacketChan()
						So(packet, ShouldResemble, rxPacket)
					})
				})
			})
		})
	})
}
