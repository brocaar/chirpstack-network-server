package node

import (
	"encoding/json"
	"testing"

	"github.com/Frankz/loraserver/api/gw"
	"github.com/Frankz/loraserver/internal/common"
	"github.com/Frankz/loraserver/internal/test"
	"github.com/Frankz/lorawan"
	. "github.com/smartystreets/goconvey/convey"
)

func TestFrameLog(t *testing.T) {
	conf := test.GetConfig()
	db, err := common.OpenDatabase(conf.PostgresDSN)
	if err != nil {
		t.Fatal(err)
	}

	Convey("Given a clean database", t, func() {
		test.MustResetDB(db)

		Convey("When creating 20 logs for two different DevEUIs", func() {
			rxInfoSet := make([]gw.RXInfo, 2)
			txInfo := gw.TXInfo{}
			devEUI1 := lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8}
			devEUI2 := lorawan.EUI64{8, 7, 6, 5, 4, 3, 2, 1}
			phy := lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MACPayload: &lorawan.MACPayload{
					FHDR: lorawan.FHDR{
						DevAddr: lorawan.DevAddr{1, 2, 3, 4},
						FCnt:    1,
					},
				},
			}

			rxBytes, err := json.Marshal(rxInfoSet)
			So(err, ShouldBeNil)
			txBytes, err := json.Marshal(txInfo)
			So(err, ShouldBeNil)
			phyBytes, err := phy.MarshalBinary()
			So(err, ShouldBeNil)

			for i := 0; i < 10; i++ {
				frameLog := FrameLog{
					DevEUI:     devEUI1,
					RXInfoSet:  &rxBytes,
					TXInfo:     &txBytes,
					PHYPayload: phyBytes,
				}
				So(CreateFrameLog(db, &frameLog), ShouldBeNil)

				frameLog.DevEUI = devEUI2
				frameLog.TXInfo = nil
				So(CreateFrameLog(db, &frameLog), ShouldBeNil)
			}

			Convey("Then getting the log count given a DevEUI returns the correct value", func() {
				count, err := GetFrameLogCountForDevEUI(db, devEUI1)
				So(err, ShouldBeNil)
				So(count, ShouldEqual, 10)
			})

			Convey("Then getting the log lines given a DevEUI returns the correct value", func() {
				logs, err := GetFrameLogsForDevEUI(db, devEUI1, 5, 0)
				So(err, ShouldBeNil)
				So(logs, ShouldHaveLength, 5)
				So(logs[0].CreatedAt, ShouldHappenAfter, logs[4].CreatedAt)
				So(logs[0].DevEUI, ShouldEqual, devEUI1)

				var rx []gw.RXInfo
				So(json.Unmarshal(*logs[0].RXInfoSet, &rx), ShouldBeNil)
				So(rx, ShouldResemble, rxInfoSet)

				var tx gw.TXInfo
				So(json.Unmarshal(*logs[0].TXInfo, &tx), ShouldBeNil)
				So(tx, ShouldResemble, txInfo)

				var p lorawan.PHYPayload
				So(p.UnmarshalBinary(logs[0].PHYPayload), ShouldBeNil)
				So(p, ShouldResemble, phy)
			})
		})
	})
}
