package mqttpubsub

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/brocaar/loraserver/models"
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
			time.Sleep(time.Millisecond * 100) // give the backend some time to subscribe to the topic

			Convey("Given the MQTT client is subscribed to gateway/+/tx", func() {
				txPacketChan := make(chan models.TXPacket)
				token := c.Subscribe("gateway/+/tx", 0, func(c mqtt.Client, msg mqtt.Message) {
					var txPacket models.TXPacket
					if err := json.Unmarshal(msg.Payload(), &txPacket); err != nil {
						t.Fatal(err)
					}
					txPacketChan <- txPacket
				})
				token.Wait()
				So(token.Error(), ShouldBeNil)

				Convey("When sending a TXPacket (from the Backend)", func() {
					txPacket := models.TXPacket{
						TXInfo: models.TXInfo{
							MAC: [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
						},
						PHYPayload: lorawan.PHYPayload{
							MHDR: lorawan.MHDR{
								MType: lorawan.UnconfirmedDataDown,
								Major: lorawan.LoRaWANR1,
							},
							MACPayload: &lorawan.MACPayload{},
						},
					}
					So(backend.SendTXPacket(txPacket), ShouldBeNil)

					Convey("Then the same packet is consumed by the MQTT client", func() {
						packet := <-txPacketChan
						So(packet, ShouldResemble, txPacket)
					})
				})

				Convey("When sending a RXPacket (from the MQTT client)", func() {
					rxPacket := models.RXPacket{
						RXInfo: models.RXInfo{
							Time: time.Now().UTC(),
							MAC:  [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
						},
						PHYPayload: lorawan.PHYPayload{
							MHDR: lorawan.MHDR{
								MType: lorawan.UnconfirmedDataUp,
								Major: lorawan.LoRaWANR1,
							},
							MACPayload: &lorawan.MACPayload{},
						},
					}
					b, err := json.Marshal(rxPacket)
					So(err, ShouldBeNil)
					token := c.Publish("gateway/0102030405060708/rx", 0, false, b)
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
