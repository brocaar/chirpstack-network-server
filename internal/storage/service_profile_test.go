package storage

import (
	"testing"
	"time"

	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/test"
	"github.com/pkg/errors"
	. "github.com/smartystreets/goconvey/convey"
)

func TestServiceProfile(t *testing.T) {
	conf := test.GetConfig()
	db, err := common.OpenDatabase(conf.PostgresDSN)
	if err != nil {
		t.Fatal(err)
	}
	config.C.PostgreSQL.DB = db
	config.C.Redis.Pool = common.NewRedisPool(conf.RedisURL, 10, 0)

	Convey("Given a clean database", t, func() {
		test.MustResetDB(config.C.PostgreSQL.DB)
		test.MustFlushRedis(config.C.Redis.Pool)

		Convey("When creating a service-profile", func() {
			sp := ServiceProfile{
				ULRate:                 1,
				ULBucketSize:           2,
				ULRatePolicy:           Drop,
				DLRate:                 3,
				DLBucketSize:           4,
				DLRatePolicy:           Mark,
				AddGWMetadata:          true,
				DevStatusReqFreq:       5,
				ReportDevStatusBattery: true,
				ReportDevStatusMargin:  true,
				DRMin:                  6,
				DRMax:                  7,
				ChannelMask:            []byte{1, 2, 3},
				PRAllowed:              true,
				HRAllowed:              true,
				RAAllowed:              true,
				NwkGeoLoc:              true,
				TargetPER:              1,
				MinGWDiversity:         8,
			}

			So(CreateServiceProfile(db, &sp), ShouldBeNil)
			sp.CreatedAt = sp.CreatedAt.UTC().Truncate(time.Millisecond)
			sp.UpdatedAt = sp.UpdatedAt.UTC().Truncate(time.Millisecond)

			Convey("Then GetServiceProfile returns the expected service-profile", func() {
				spGet, err := GetServiceProfile(db, sp.ID)
				So(err, ShouldBeNil)

				spGet.CreatedAt = spGet.CreatedAt.UTC().Truncate(time.Millisecond)
				spGet.UpdatedAt = spGet.UpdatedAt.UTC().Truncate(time.Millisecond)
				So(spGet, ShouldResemble, sp)
			})

			Convey("Then UpdateServiceProfile updates the service-profile", func() {
				sp.ULRate = 2
				sp.ULBucketSize = 3
				sp.ULRatePolicy = Mark
				sp.DLRate = 4
				sp.DLBucketSize = 5
				sp.DLRatePolicy = Drop
				sp.AddGWMetadata = false
				sp.DevStatusReqFreq = 6
				sp.ReportDevStatusBattery = false
				sp.ReportDevStatusMargin = false
				sp.DRMin = 7
				sp.DRMax = 8
				sp.ChannelMask = []byte{3, 2, 1}
				sp.PRAllowed = false
				sp.HRAllowed = false
				sp.RAAllowed = false
				sp.NwkGeoLoc = false
				sp.TargetPER = 2
				sp.MinGWDiversity = 9

				So(UpdateServiceProfile(db, &sp), ShouldBeNil)
				sp.UpdatedAt = sp.UpdatedAt.UTC().Truncate(time.Millisecond)

				spGet, err := GetServiceProfile(db, sp.ID)
				So(err, ShouldBeNil)

				spGet.CreatedAt = spGet.CreatedAt.UTC().Truncate(time.Millisecond)
				spGet.UpdatedAt = spGet.UpdatedAt.UTC().Truncate(time.Millisecond)
				So(spGet, ShouldResemble, sp)
			})

			Convey("Then DeleteServiceProfile deletes the service-profile", func() {
				So(DeleteServiceProfile(db, sp.ID), ShouldBeNil)
				So(DeleteServiceProfile(db, sp.ID), ShouldEqual, ErrDoesNotExist)
			})

			Convey("Then GetAndCacheServiceProfile reads the service-profile from db and puts it in cache", func() {
				spGet, err := GetAndCacheServiceProfile(config.C.PostgreSQL.DB, config.C.Redis.Pool, sp.ID)
				So(err, ShouldBeNil)
				So(spGet.ID, ShouldEqual, sp.ID)

				Convey("Then GetServiceProfileCache returns the service-profile", func() {
					spGet, err := GetServiceProfileCache(config.C.Redis.Pool, sp.ID)
					So(err, ShouldBeNil)
					So(spGet.ID, ShouldEqual, sp.ID)
				})

				Convey("Then FlushServiceProfileCache removes the service-profile from cache", func() {
					err := FlushServiceProfileCache(config.C.Redis.Pool, sp.ID)
					So(err, ShouldBeNil)

					_, err = GetServiceProfileCache(config.C.Redis.Pool, sp.ID)
					So(err, ShouldNotBeNil)
					So(errors.Cause(err), ShouldEqual, ErrDoesNotExist)
				})
			})
		})
	})
}
