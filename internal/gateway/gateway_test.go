package gateway

import (
	"errors"
	"testing"
	"time"

	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/test"
	"github.com/brocaar/lorawan"
	. "github.com/smartystreets/goconvey/convey"
)

func TestGatewayFunctions(t *testing.T) {
	conf := test.GetConfig()

	Convey("Given a clean database", t, func() {
		db, err := common.OpenDatabase(conf.PostgresDSN)
		So(err, ShouldBeNil)
		test.MustResetDB(db)

		Convey("When creating a gateway", func() {
			gw := Gateway{
				MAC: lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
				Location: GPSPoint{
					Latitude:  1.23456789,
					Longitude: 4.56789012,
				},
				Altitude:            100,
				RXPacketsReceived:   100,
				RXPacketsReceivedOK: 80,
				TXPacketsReceived:   200,
				TXPacketsEmitted:    180,
			}
			So(CreateGateway(db, &gw), ShouldBeNil)

			// some precicion will get lost when writing to the db
			// truncate it to ms precision for comparison
			gw.CratedAt = gw.CratedAt.UTC().Truncate(time.Millisecond)
			gw.UpdatedAt = gw.UpdatedAt.UTC().Truncate(time.Millisecond)

			Convey("Then it can be retrieved", func() {
				gw2, err := GetGateway(db, gw.MAC)
				So(err, ShouldBeNil)

				gw2.CratedAt = gw2.CratedAt.UTC().Truncate(time.Millisecond)
				gw2.UpdatedAt = gw2.UpdatedAt.UTC().Truncate(time.Millisecond)

				So(gw2, ShouldResemble, gw)
			})

			Convey("Then it can be updated", func() {
				gw.Location.Latitude = 1.987654321
				gw.Location.Longitude = 2.987654321
				gw.Altitude++
				gw.RXPacketsReceived++
				gw.RXPacketsReceivedOK++
				gw.TXPacketsReceived++
				gw.TXPacketsEmitted++

				So(UpdateGateway(db, &gw), ShouldBeNil)
				gw.UpdatedAt = gw.UpdatedAt.UTC().Truncate(time.Millisecond)

				gw2, err := GetGateway(db, gw.MAC)
				So(err, ShouldBeNil)
				gw2.CratedAt = gw2.CratedAt.UTC().Truncate(time.Millisecond)
				gw2.UpdatedAt = gw2.UpdatedAt.UTC().Truncate(time.Millisecond)
				So(gw2, ShouldResemble, gw)
			})

			Convey("Then the gateway count is 1", func() {
				count, err := GetGatewayCount(db)
				So(err, ShouldBeNil)
				So(count, ShouldEqual, 1)
			})

			Convey("Then listing the gateways returns the expected item", func() {
				gws, err := GetGateways(db, 10, 0)
				So(err, ShouldBeNil)
				So(gws, ShouldHaveLength, 1)

				gws[0].CratedAt = gws[0].CratedAt.UTC().Truncate(time.Millisecond)
				gws[0].UpdatedAt = gws[0].UpdatedAt.UTC().Truncate(time.Millisecond)
				So(gws[0], ShouldResemble, gw)
			})

			Convey("Then it can be deleted", func() {
				So(DeleteGateway(db, gw.MAC), ShouldBeNil)
				_, err := GetGateway(db, gw.MAC)
				So(err, ShouldResemble, errors.New("get gateway error: sql: no rows in result set"))
			})
		})
	})
}
