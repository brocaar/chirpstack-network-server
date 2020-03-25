package storage

import (
	"context"
	"testing"
	"time"

	"github.com/pkg/errors"
	. "github.com/smartystreets/goconvey/convey"

	"github.com/brocaar/chirpstack-network-server/internal/test"
)

func TestDeviceProfile(t *testing.T) {
	conf := test.GetConfig()
	if err := Setup(conf); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	Convey("Given a clean database", t, func() {
		test.MustResetDB(DB().DB)
		RedisClient().FlushAll()

		Convey("When creating a device-profile", func() {
			dp := DeviceProfile{
				SupportsClassB:      true,
				ClassBTimeout:       1,
				PingSlotPeriod:      2,
				PingSlotDR:          3,
				PingSlotFreq:        868100000,
				SupportsClassC:      true,
				ClassCTimeout:       4,
				MACVersion:          "1.0.2",
				RegParamsRevision:   "B",
				RXDelay1:            5,
				RXDROffset1:         6,
				RXDataRate2:         7,
				RXFreq2:             868200000,
				FactoryPresetFreqs:  []int{868400000, 868500000, 868700000},
				MaxEIRP:             17,
				MaxDutyCycle:        10,
				SupportsJoin:        true,
				RFRegion:            "EU868",
				Supports32bitFCnt:   true,
				GeolocBufferTTL:     10,
				GeolocMinBufferSize: 3,
			}

			So(CreateDeviceProfile(context.Background(), DB(), &dp), ShouldBeNil)
			dp.CreatedAt = dp.CreatedAt.UTC().Truncate(time.Millisecond)
			dp.UpdatedAt = dp.UpdatedAt.UTC().Truncate(time.Millisecond)

			Convey("Then GetDeviceProfile returns the expected device-profile", func() {
				dpGet, err := GetDeviceProfile(ctx, DB(), dp.ID)
				So(err, ShouldBeNil)

				dpGet.CreatedAt = dpGet.CreatedAt.UTC().Truncate(time.Millisecond)
				dpGet.UpdatedAt = dpGet.UpdatedAt.UTC().Truncate(time.Millisecond)
				So(dpGet, ShouldResemble, dp)
			})

			Convey("Then DeleteDeviceProfile deletes the device-profile", func() {
				So(DeleteDeviceProfile(context.Background(), DB(), dp.ID), ShouldBeNil)
				So(DeleteDeviceProfile(context.Background(), DB(), dp.ID), ShouldEqual, ErrDoesNotExist)
			})

			Convey("Then GetAndCacheDeviceProfile reads the device-profile from db and puts it in cache", func() {
				dpGet, err := GetAndCacheDeviceProfile(ctx, DB(), dp.ID)
				So(err, ShouldBeNil)
				So(dpGet.ID, ShouldEqual, dp.ID)

				Convey("Then GetDeviceProfileCache returns the device-profile", func() {
					dpGet, err := GetDeviceProfileCache(context.Background(), dp.ID)
					So(err, ShouldBeNil)
					So(dpGet.ID, ShouldEqual, dp.ID)
				})

				Convey("Then FlushDeviceProfileCache removes the device-profile from cache", func() {
					err := FlushDeviceProfileCache(context.Background(), dp.ID)
					So(err, ShouldBeNil)

					_, err = GetDeviceProfileCache(context.Background(), dp.ID)
					So(err, ShouldNotBeNil)
					So(errors.Cause(err), ShouldEqual, ErrDoesNotExist)
				})
			})

			Convey("Then UpdateDeviceProfile updates the device-profile", func() {
				dp.SupportsClassB = false
				dp.ClassBTimeout = 2
				dp.PingSlotPeriod = 3
				dp.PingSlotDR = 4
				dp.PingSlotFreq = 868200000
				dp.SupportsClassC = false
				dp.ClassCTimeout = 5
				dp.MACVersion = "1.1.0"
				dp.RegParamsRevision = "C"
				dp.RXDelay1 = 6
				dp.RXDROffset1 = 7
				dp.RXDataRate2 = 8
				dp.RXFreq2 = 868300000
				dp.FactoryPresetFreqs = []int{868400000, 868500000, 868700000}
				dp.MaxEIRP = 14
				dp.MaxDutyCycle = 1
				dp.SupportsJoin = false
				dp.RFRegion = "US902"
				dp.Supports32bitFCnt = false
				dp.GeolocBufferTTL = 20
				dp.GeolocMinBufferSize = 4

				So(UpdateDeviceProfile(context.Background(), DB(), &dp), ShouldBeNil)
				dp.UpdatedAt = dp.UpdatedAt.UTC().Truncate(time.Millisecond)

				dpGet, err := GetDeviceProfile(ctx, DB(), dp.ID)
				So(err, ShouldBeNil)

				dpGet.CreatedAt = dpGet.CreatedAt.UTC().Truncate(time.Millisecond)
				dpGet.UpdatedAt = dpGet.UpdatedAt.UTC().Truncate(time.Millisecond)
				So(dpGet, ShouldResemble, dp)
			})
		})
	})
}
