package loraserver

import (
	"errors"
	"testing"
	"time"

	"github.com/brocaar/lorawan"
	. "github.com/smartystreets/goconvey/convey"
)

func TestHandleRXPackets(t *testing.T) {
	conf := getConfig()

	Convey("Given a dummy gateway and application backend and a clean Redis database", t, func() {
		app := &testApplicationBackend{
			rxPacketsChan: make(chan RXPackets, 1),
		}
		gw := &testGatewayBackend{
			rxPacketChan: make(chan RXPacket),
		}
		p := NewRedisPool(conf.RedisURL)
		mustFlushRedis(p)

		ctx := Context{
			RedisPool:   p,
			Gateway:     gw,
			Application: app,
		}

		Convey("Given a stored node session", func() {
			ns := NodeSession{
				DevAddr:  lorawan.DevAddr{1, 2, 3, 4},
				DevEUI:   lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
				AppSKey:  lorawan.AES128Key{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
				NwkSKey:  lorawan.AES128Key{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
				FCntUp:   10,
				FCntDown: 0,

				AppEUI: lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1},
				AppKey: lorawan.AES128Key{1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2},
			}
			So(SaveNodeSession(p, ns), ShouldBeNil)

			Convey("Given UnconfirmedDataUp packet", func() {
				macPL := lorawan.NewMACPayload(true)
				macPL.FHDR = lorawan.FHDR{
					DevAddr: ns.DevAddr,
					FCnt:    10,
				}
				macPL.FPort = 1
				macPL.FRMPayload = []lorawan.Payload{
					&lorawan.DataPayload{Bytes: []byte("hello!")},
				}
				So(macPL.EncryptFRMPayload(ns.AppSKey), ShouldBeNil)

				phy := lorawan.NewPHYPayload(true)
				phy.MHDR = lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				}
				phy.MACPayload = macPL
				So(phy.SetMIC(ns.NwkSKey), ShouldBeNil)

				rxPacket := RXPacket{
					RXInfo: RXInfo{
						MAC:        [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
						Time:       time.Now().UTC(),
						Timestamp:  708016819,
						Frequency:  868.5,
						Channel:    2,
						RFChain:    1,
						CRCStatus:  1,
						Modulation: "LORA",
						DataRate:   DataRate{LoRa: "SF7BW125"},
						CodeRate:   "4/5",
						RSSI:       -51,
						LoRaSNR:    7,
						Size:       16,
					},
					PHYPayload: phy,
				}

				Convey("When calling handleRXPacket", func() {
					So(handleRXPacket(ctx, rxPacket), ShouldBeNil)

					Convey("Then the packet is correctly received by the application backend", func() {
						packets := <-app.rxPacketsChan
						phy := packets[0].PHYPayload
						mac := phy.MACPayload.(*lorawan.MACPayload)
						data := mac.FRMPayload[0].(*lorawan.DataPayload)
						So(data.Bytes, ShouldResemble, []byte("hello!"))
					})

					Convey("Then the FCntUp has been incremented", func() {
						nsUpdated, err := GetNodeSession(p, ns.DevAddr)
						So(err, ShouldBeNil)
						So(nsUpdated.FCntUp, ShouldEqual, ns.FCntUp+1)
					})
				})

				Convey("Given that the application backend returns an error", func() {
					app.err = errors.New("BOOM")

					Convey("When calling handleRXPacket", func() {
						So(handleRXPacket(ctx, rxPacket), ShouldNotBeNil)

						Convey("Then the FCntUp has not been incremented", func() {
							nsUpdated, err := GetNodeSession(p, ns.DevAddr)
							So(err, ShouldBeNil)
							So(nsUpdated, ShouldResemble, ns)
						})
					})
				})

				Convey("When the frame-counter is invalid", func() {
					ns.FCntUp = 11
					So(SaveNodeSession(p, ns), ShouldBeNil)

					Convey("Then handleRXPacket returns a frame-counter related error", func() {
						err := handleRXPacket(ctx, rxPacket)
						So(err, ShouldResemble, errors.New("invalid FCnt or too many dropped frames"))
					})
				})

				Convey("When the MIC is invalid", func() {
					rxPacket.PHYPayload.MIC = [4]byte{1, 2, 3, 4}

					Convey("Then handleRXPacket returns a MIC related error", func() {
						err := handleRXPacket(ctx, rxPacket)
						So(err, ShouldResemble, errors.New("invalid MIC"))
					})
				})
			})
		})
	})
}
