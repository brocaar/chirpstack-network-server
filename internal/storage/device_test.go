package storage

import (
	"testing"
	"time"

	"github.com/brocaar/lorawan"

	"github.com/satori/go.uuid"

	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/test"
	. "github.com/smartystreets/goconvey/convey"
)

func TestDevice(t *testing.T) {
	conf := test.GetConfig()
	db, err := common.OpenDatabase(conf.PostgresDSN)
	if err != nil {
		t.Fatal(err)
	}
	common.DB = db

	Convey("Given a clean database", t, func() {
		test.MustResetDB(common.DB)

		Convey("Given a service, device and routing profile", func() {
			createdByID := uuid.NewV4().String()

			sp := ServiceProfile{
				CreatedBy: createdByID,
			}
			So(CreateServiceProfile(db, &sp), ShouldBeNil)

			dp := DeviceProfile{
				CreatedBy: createdByID,
			}
			So(CreateDeviceProfile(db, &dp), ShouldBeNil)

			rp := RoutingProfile{
				CreatedBy: createdByID,
			}
			So(CreateRoutingProfile(db, &rp), ShouldBeNil)

			Convey("When creating a device", func() {
				d := Device{
					DevEUI:           lorawan.EUI64{1, 2, 3, 4, 5, 6, 7, 8},
					CreatedBy:        createdByID,
					ServiceProfileID: sp.ServiceProfile.ServiceProfileID,
					DeviceProfileID:  dp.DeviceProfile.DeviceProfileID,
					RoutingProfileID: rp.RoutingProfile.RoutingProfileID,
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
					spNew := ServiceProfile{
						CreatedBy: createdByID,
					}
					So(CreateServiceProfile(db, &spNew), ShouldBeNil)

					dpNew := DeviceProfile{
						CreatedBy: createdByID,
					}
					So(CreateDeviceProfile(db, &dpNew), ShouldBeNil)

					rpNew := RoutingProfile{
						CreatedBy: createdByID,
					}
					So(CreateRoutingProfile(db, &rpNew), ShouldBeNil)

					d.ServiceProfileID = spNew.ServiceProfile.ServiceProfileID
					d.DeviceProfileID = dpNew.DeviceProfile.DeviceProfileID
					d.RoutingProfileID = rpNew.RoutingProfile.RoutingProfileID
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
						DevEUI:   d.DevEUI,
						JoinEUI:  joinEUI,
						DevAddr:  lorawan.DevAddr{1, 2, 3, 4},
						NwkSKey:  lorawan.AES128Key{1, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8},
						DevNonce: lorawan.DevNonce{1, 2},
					}
					So(CreateDeviceActivation(common.DB, &da), ShouldBeNil)

					Convey("Then GetLastDeviceActivationForDevEUI returns the most recent device-activation", func() {
						da2 := DeviceActivation{
							DevEUI:   d.DevEUI,
							JoinEUI:  joinEUI,
							DevAddr:  lorawan.DevAddr{4, 3, 2, 1},
							NwkSKey:  lorawan.AES128Key{8, 7, 6, 5, 4, 3, 2, 1, 8, 7, 6, 5, 4, 3, 2, 1},
							DevNonce: lorawan.DevNonce{2, 1},
						}
						So(CreateDeviceActivation(common.DB, &da2), ShouldBeNil)
						da2.CreatedAt = da2.CreatedAt.UTC().Truncate(time.Millisecond)

						daGet, err := GetLastDeviceActivationForDevEUI(common.DB, d.DevEUI)
						So(err, ShouldBeNil)
						daGet.CreatedAt = daGet.CreatedAt.UTC().Truncate(time.Millisecond)
						So(daGet, ShouldResemble, da2)
					})

					Convey("Then ValidateDevNonce for an used dev-nonce returns an error", func() {
						So(ValidateDevNonce(common.DB, joinEUI, d.DevEUI, da.DevNonce), ShouldEqual, ErrAlreadyExists)
					})

					Convey("Then ValidateDevNonce for an unused dev-nonce returns no error", func() {
						So(ValidateDevNonce(common.DB, joinEUI, d.DevEUI, lorawan.DevNonce{2, 1}), ShouldBeNil)
					})
				})
			})
		})
	})
}
