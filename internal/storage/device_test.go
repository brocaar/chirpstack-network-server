package storage

import (
	"testing"
	"time"

	"github.com/brocaar/lorawan"

	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/test"
	. "github.com/smartystreets/goconvey/convey"
)

func TestDevice(t *testing.T) {
	conf := test.GetConfig()
	db, err := common.OpenDatabase(conf.PostgresDSN)
	if err != nil {
		t.Fatal(err)
	}
	config.C.PostgreSQL.DB = db

	Convey("Given a clean database", t, func() {
		test.MustResetDB(config.C.PostgreSQL.DB)

		Convey("Given a service, device and routing profile", func() {
			sp := ServiceProfile{}
			So(CreateServiceProfile(db, &sp), ShouldBeNil)

			dp := DeviceProfile{}
			So(CreateDeviceProfile(db, &dp), ShouldBeNil)

			rp := RoutingProfile{}
			So(CreateRoutingProfile(db, &rp), ShouldBeNil)

			Convey("When creating a device", func() {
				d := Device{
					DevEUI:           lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
					ServiceProfileID: sp.ID,
					DeviceProfileID:  dp.ID,
					RoutingProfileID: rp.ID,
					SkipFCntCheck:    true,
				}
				So(CreateDevice(db, &d), ShouldBeNil)
				d.CreatedAt = d.CreatedAt.UTC().Truncate(time.Millisecond)
				d.UpdatedAt = d.UpdatedAt.UTC().Truncate(time.Millisecond)

				Convey("Then GetDevice returns the expected device", func() {
					dGet, err := GetDevice(db, d.DevEUI)
					So(err, ShouldBeNil)

					dGet.CreatedAt = dGet.CreatedAt.UTC().Truncate(time.Millisecond)
					dGet.UpdatedAt = dGet.UpdatedAt.UTC().Truncate(time.Millisecond)
					So(dGet, ShouldResemble, d)
				})

				Convey("Then UpdateDevice updates the device", func() {
					spNew := ServiceProfile{}
					So(CreateServiceProfile(db, &spNew), ShouldBeNil)

					dpNew := DeviceProfile{}
					So(CreateDeviceProfile(db, &dpNew), ShouldBeNil)

					rpNew := RoutingProfile{}
					So(CreateRoutingProfile(db, &rpNew), ShouldBeNil)

					d.ServiceProfileID = spNew.ID
					d.DeviceProfileID = dpNew.ID
					d.RoutingProfileID = rpNew.ID
					d.SkipFCntCheck = false
					So(UpdateDevice(db, &d), ShouldBeNil)
					d.UpdatedAt = d.UpdatedAt.UTC().Truncate(time.Millisecond)

					dGet, err := GetDevice(db, d.DevEUI)
					So(err, ShouldBeNil)

					dGet.CreatedAt = dGet.CreatedAt.UTC().Truncate(time.Millisecond)
					dGet.UpdatedAt = dGet.UpdatedAt.UTC().Truncate(time.Millisecond)
					So(dGet, ShouldResemble, d)
				})

				Convey("Then DeleteDevice deletes the device", func() {
					So(DeleteDevice(db, d.DevEUI), ShouldBeNil)
					So(DeleteDevice(db, d.DevEUI), ShouldEqual, ErrDoesNotExist)
				})

				Convey("Then CreateDeviceActivation creates the device-activation", func() {
					joinEUI := lorawan.EUI64{1, 2, 1, 2, 1, 2, 1, 2}

					da := DeviceActivation{
						DevEUI:      d.DevEUI,
						JoinEUI:     joinEUI,
						DevAddr:     lorawan.DevAddr{1, 2, 3, 4},
						SNwkSIntKey: lorawan.AES128Key{1, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8},
						FNwkSIntKey: lorawan.AES128Key{2, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8},
						NwkSEncKey:  lorawan.AES128Key{3, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8},
						DevNonce:    258,
						JoinReqType: lorawan.JoinRequestType,
					}
					So(CreateDeviceActivation(config.C.PostgreSQL.DB, &da), ShouldBeNil)

					Convey("Then GetLastDeviceActivationForDevEUI returns the most recent device-activation", func() {
						da2 := DeviceActivation{
							DevEUI:      d.DevEUI,
							JoinEUI:     joinEUI,
							DevAddr:     lorawan.DevAddr{4, 3, 2, 1},
							SNwkSIntKey: lorawan.AES128Key{8, 7, 6, 5, 4, 3, 2, 1, 8, 7, 6, 5, 4, 3, 2, 1},
							FNwkSIntKey: lorawan.AES128Key{8, 7, 6, 5, 4, 3, 2, 1, 8, 7, 6, 5, 4, 3, 2, 2},
							NwkSEncKey:  lorawan.AES128Key{8, 7, 6, 5, 4, 3, 2, 1, 8, 7, 6, 5, 4, 3, 2, 3},
							DevNonce:    513,
							JoinReqType: lorawan.JoinRequestType,
						}
						So(CreateDeviceActivation(config.C.PostgreSQL.DB, &da2), ShouldBeNil)
						da2.CreatedAt = da2.CreatedAt.UTC().Truncate(time.Millisecond)

						daGet, err := GetLastDeviceActivationForDevEUI(config.C.PostgreSQL.DB, d.DevEUI)
						So(err, ShouldBeNil)
						daGet.CreatedAt = daGet.CreatedAt.UTC().Truncate(time.Millisecond)
						So(daGet, ShouldResemble, da2)
					})

					Convey("Then ValidateDevNonce for an used dev-nonce returns an error", func() {
						So(ValidateDevNonce(config.C.PostgreSQL.DB, joinEUI, d.DevEUI, da.DevNonce, lorawan.JoinRequestType), ShouldEqual, ErrAlreadyExists)
					})

					Convey("Then ValidateDevNonce for an unused dev-nonce returns no error", func() {
						So(ValidateDevNonce(config.C.PostgreSQL.DB, joinEUI, d.DevEUI, lorawan.DevNonce(513), lorawan.JoinRequestType), ShouldBeNil)
					})
				})
			})
		})
	})
}
