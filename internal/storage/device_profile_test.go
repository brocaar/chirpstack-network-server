package storage

import (
	"testing"
	"time"

	"github.com/Frankz/loraserver/internal/common"
	"github.com/Frankz/loraserver/internal/test"
	"github.com/Frankz/lorawan/backend"
	"github.com/pkg/errors"
	. "github.com/smartystreets/goconvey/convey"
)

func TestDeviceProfile(t *testing.T) {
	conf := test.GetConfig()
	db, err := common.OpenDatabase(conf.PostgresDSN)
	if err != nil {
		t.Fatal(err)
	}
	common.DB = db
	common.RedisPool = common.NewRedisPool(conf.RedisURL)

	Convey("Given a clean database", t, func() {
		test.MustResetDB(common.DB)
		test.MustFlushRedis(common.RedisPool)

		Convey("When creating a device-profile", func() {
			dp := DeviceProfile{
				DeviceProfile: backend.DeviceProfile{
					SupportsClassB:     true,
					ClassBTimeout:      1,
					PingSlotPeriod:     2,
					PingSlotDR:         3,
					PingSlotFreq:       868100000,
					SupportsClassC:     true,
					ClassCTimeout:      4,
					MACVersion:         "1.0.2",
					RegParamsRevision:  "B",
					RXDelay1:           5,
					RXDROffset1:        6,
					RXDataRate2:        7,
					RXFreq2:            868200000,
					FactoryPresetFreqs: []backend.Frequency{868400000, 868500000, 868700000},
					MaxEIRP:            17,
					MaxDutyCycle:       10,
					SupportsJoin:       true,
					RFRegion:           backend.EU868,
					Supports32bitFCnt:  true,
				},
			}

			So(CreateDeviceProfile(db, &dp), ShouldBeNil)
			dp.CreatedAt = dp.CreatedAt.UTC().Truncate(time.Millisecond)
			dp.UpdatedAt = dp.UpdatedAt.UTC().Truncate(time.Millisecond)

			Convey("Then GetDeviceProfile returns the expected device-profile", func() {
				dpGet, err := GetDeviceProfile(db, dp.DeviceProfile.DeviceProfileID)
				So(err, ShouldBeNil)

				dpGet.CreatedAt = dpGet.CreatedAt.UTC().Truncate(time.Millisecond)
				dpGet.UpdatedAt = dpGet.UpdatedAt.UTC().Truncate(time.Millisecond)
				So(dpGet, ShouldResemble, dp)
			})

			Convey("Then DeleteDeviceProfile deletes the device-profile", func() {
				So(DeleteDeviceProfile(db, dp.DeviceProfile.DeviceProfileID), ShouldBeNil)
				So(DeleteDeviceProfile(db, dp.DeviceProfile.DeviceProfileID), ShouldEqual, ErrDoesNotExist)
			})

			Convey("Then GetAndCacheDeviceProfile reads the device-profile from db and puts it in cache", func() {
				dpGet, err := GetAndCacheDeviceProfile(common.DB, common.RedisPool, dp.DeviceProfile.DeviceProfileID)
				So(err, ShouldBeNil)
				So(dpGet.DeviceProfile.DeviceProfileID, ShouldEqual, dp.DeviceProfile.DeviceProfileID)

				Convey("Then GetDeviceProfileCache returns the device-profile", func() {
					dpGet, err := GetDeviceProfileCache(common.RedisPool, dp.DeviceProfile.DeviceProfileID)
					So(err, ShouldBeNil)
					So(dpGet.DeviceProfile.DeviceProfileID, ShouldEqual, dp.DeviceProfile.DeviceProfileID)
				})

				Convey("Then FlushDeviceProfileCache removes the device-profile from cache", func() {
					err := FlushDeviceProfileCache(common.RedisPool, dp.DeviceProfile.DeviceProfileID)
					So(err, ShouldBeNil)

					_, err = GetDeviceProfileCache(common.RedisPool, dp.DeviceProfile.DeviceProfileID)
					So(err, ShouldNotBeNil)
					So(errors.Cause(err), ShouldEqual, ErrDoesNotExist)
				})
			})

			Convey("Then UpdateDeviceProfile updates the device-profile", func() {
				dp.DeviceProfile = backend.DeviceProfile{
					DeviceProfileID:    dp.DeviceProfile.DeviceProfileID,
					SupportsClassB:     false,
					ClassBTimeout:      2,
					PingSlotPeriod:     3,
					PingSlotDR:         4,
					PingSlotFreq:       868200000,
					SupportsClassC:     false,
					ClassCTimeout:      5,
					MACVersion:         "1.1.0",
					RegParamsRevision:  "C",
					RXDelay1:           6,
					RXDROffset1:        7,
					RXDataRate2:        8,
					RXFreq2:            868300000,
					FactoryPresetFreqs: []backend.Frequency{868400000, 868500000, 868700000},
					MaxEIRP:            14,
					MaxDutyCycle:       1,
					SupportsJoin:       false,
					RFRegion:           backend.US902,
					Supports32bitFCnt:  false,
				}
				So(UpdateDeviceProfile(db, &dp), ShouldBeNil)
				dp.UpdatedAt = dp.UpdatedAt.UTC().Truncate(time.Millisecond)

				dpGet, err := GetDeviceProfile(db, dp.DeviceProfile.DeviceProfileID)
				So(err, ShouldBeNil)

				dpGet.CreatedAt = dpGet.CreatedAt.UTC().Truncate(time.Millisecond)
				dpGet.UpdatedAt = dpGet.UpdatedAt.UTC().Truncate(time.Millisecond)
				So(dpGet, ShouldResemble, dp)
			})
		})
	})
}
