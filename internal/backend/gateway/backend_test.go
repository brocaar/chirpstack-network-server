package gateway

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/lorawan"
	"github.com/eclipse/paho.mqtt.golang"
	. "github.com/smartystreets/goconvey/convey"
)

func TestBackend(t *testing.T) {
	conf := getConfig()
	p := common.NewRedisPool(conf.RedisURL)

	Convey("Given a MQTT client", t, func() {
		opts := mqtt.NewClientOptions().AddBroker(conf.Server).SetUsername(conf.Username).SetPassword(conf.Password)
		c := mqtt.NewClient(opts)
		token := c.Connect()
		token.Wait()
		So(token.Error(), ShouldBeNil)

		Convey("Given a new Backend", func() {
			MustFlushRedis(p)
			backend, err := NewMQTTBackend(
				p,
				MQTTBackendConfig{
					Server:                conf.Server,
					Username:              conf.Username,
					Password:              conf.Password,
					CleanSession:          true,
					UplinkTopicTemplate:   "gateway/+/rx",
					DownlinkTopicTemplate: "gateway/{{ .MAC }}/tx",
					StatsTopicTemplate:    "gateway/+/stats",
					AckTopicTemplate:      "gateway/+/ack",
				},
			)
			So(err, ShouldBeNil)
			defer backend.Close()
			time.Sleep(time.Millisecond * 100) // give the backend some time to subscribe to the topic

			Convey("Given the MQTT client is subscribed to gateway/+/tx and gateway/+/stats", func() {
				statsPacketChan := make(chan gw.GatewayStatsPacket)
				txPacketChan := make(chan gw.TXPacket)

				token := c.Subscribe("gateway/+/tx", 0, func(c mqtt.Client, msg mqtt.Message) {
					var txPacket gw.TXPacket
					if err := json.Unmarshal(msg.Payload(), &txPacket); err != nil {
						t.Fatal(err)
					}
					txPacketChan <- txPacket
				})
				token.Wait()
				So(token.Error(), ShouldBeNil)

				token = c.Subscribe("gateway/+/stats", 0, func(c mqtt.Client, msg mqtt.Message) {
					var statsPacket gw.GatewayStatsPacket
					if err := json.Unmarshal(msg.Payload(), &statsPacket); err != nil {
						t.Fatal(err)
					}
					statsPacketChan <- statsPacket
				})
				token.Wait()
				So(token.Error(), ShouldBeNil)

				Convey("Given a TXPacket", func() {
					txPacket := gw.TXPacket{
						TXInfo: gw.TXInfo{
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

					Convey("When sending it from the backend", func() {
						So(backend.SendTXPacket(txPacket), ShouldBeNil)

						Convey("Then the same packet has been received", func() {
							packet := <-txPacketChan
							So(packet, ShouldResemble, txPacket)
						})
					})

				})

				Convey("Given a GatewayStatsPacket", func() {
					statsPacket := gw.GatewayStatsPacket{
						MAC:  lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
						Time: time.Time{}.UTC(),
					}

					Convey("When sending it once", func() {
						b, err := json.Marshal(statsPacket)
						So(err, ShouldBeNil)
						token := c.Publish("gateway/0102030405060708/stats", 0, false, b)
						token.Wait()
						So(token.Error(), ShouldBeNil)

						Convey("Then the same packet is consumed by the backend", func() {
							packet := <-backend.StatsPacketChan()
							So(packet, ShouldResemble, statsPacket)
						})
					})

					Convey("When sending it twice with the same MAC", func() {
						for i := 0; i < 2; i++ {
							b, err := json.Marshal(statsPacket)
							So(err, ShouldBeNil)
							token := c.Publish("gateway/0102030405060708/stats", 0, false, b)
							token.Wait()
							So(token.Error(), ShouldBeNil)
						}

						Convey("Then it is received only once by the backend", func() {
							<-backend.StatsPacketChan()

							var received bool
							select {
							case <-backend.StatsPacketChan():
								received = true
							case <-time.After(time.Millisecond * 100):
							}
							So(received, ShouldBeFalse)
						})
					})
				})

				Convey("Given an RXPacket", func() {
					now := time.Now().UTC()
					rxPacket := gw.RXPacket{
						RXInfo: gw.RXInfo{
							Time: &now,
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
					phyB, err := rxPacket.PHYPayload.MarshalBinary()
					So(err, ShouldBeNil)

					Convey("When sending it once", func() {
						b, err := json.Marshal(gw.RXPacketBytes{
							RXInfo:     rxPacket.RXInfo,
							PHYPayload: phyB,
						})
						So(err, ShouldBeNil)
						token := c.Publish("gateway/0102030405060708/rx", 0, false, b)
						token.Wait()
						So(token.Error(), ShouldBeNil)

						Convey("Then the same packet is consumed by the backend", func() {
							packet := <-backend.RXPacketChan()
							So(packet, ShouldResemble, rxPacket)
						})
					})

					Convey("When sending it twice with the same MAC", func() {
						b, err := json.Marshal(gw.RXPacketBytes{
							RXInfo:     rxPacket.RXInfo,
							PHYPayload: phyB,
						})
						So(err, ShouldBeNil)
						token := c.Publish("gateway/0102030405060708/rx", 0, false, b)
						token.Wait()
						So(token.Error(), ShouldBeNil)
						token = c.Publish("gateway/0102030405060708/rx", 0, false, b)
						token.Wait()
						So(token.Error(), ShouldBeNil)

						Convey("Then it is received only once", func() {
							<-backend.RXPacketChan()

							var received bool
							select {
							case <-backend.RXPacketChan():
								received = true
							case <-time.After(time.Millisecond * 100):
							}
							So(received, ShouldBeFalse)
						})
					})

					Convey("When sending it twice with different MACs", func() {
						b, err := json.Marshal(gw.RXPacketBytes{
							RXInfo:     rxPacket.RXInfo,
							PHYPayload: phyB,
						})
						So(err, ShouldBeNil)
						token := c.Publish("gateway/0102030405060708/rx", 0, false, b)
						token.Wait()

						rxPacket.RXInfo.MAC = [8]byte{8, 7, 6, 5, 4, 3, 2, 1}
						b, err = json.Marshal(gw.RXPacketBytes{
							RXInfo:     rxPacket.RXInfo,
							PHYPayload: phyB,
						})
						So(err, ShouldBeNil)
						token = c.Publish("gateway/0102030405060708/rx", 0, false, b)
						token.Wait()

						Convey("Then it is received twice", func() {
							<-backend.RXPacketChan()
							<-backend.RXPacketChan()
						})
					})
				})
			})
		})
	})
}
