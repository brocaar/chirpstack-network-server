package storage

import (
	"testing"
	"time"

	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/test"
	"github.com/gofrs/uuid"
	. "github.com/smartystreets/goconvey/convey"
)

func TestRoutingProfile(t *testing.T) {
	conf := test.GetConfig()
	db, err := common.OpenDatabase(conf.PostgresDSN)
	if err != nil {
		t.Fatal(err)
	}
	config.C.PostgreSQL.DB = db

	Convey("Given a clean database", t, func() {
		test.MustResetDB(config.C.PostgreSQL.DB)

		Convey("When creating a routing-profile", func() {
			rp := RoutingProfile{
				ASID:    "application-server:1234",
				CACert:  "CACERT",
				TLSCert: "TLSCERT",
				TLSKey:  "TLSKEY",
			}
			So(CreateRoutingProfile(db, &rp), ShouldBeNil)
			rp.CreatedAt = rp.CreatedAt.UTC().Truncate(time.Millisecond)
			rp.UpdatedAt = rp.UpdatedAt.UTC().Truncate(time.Millisecond)

			Convey("Then GetRoutingProfile returns the expected routing-profile", func() {
				rpGet, err := GetRoutingProfile(db, rp.ID)
				So(err, ShouldBeNil)

				rpGet.CreatedAt = rpGet.CreatedAt.UTC().Truncate(time.Millisecond)
				rpGet.UpdatedAt = rpGet.UpdatedAt.UTC().Truncate(time.Millisecond)
				So(rpGet, ShouldResemble, rp)
				So(rpGet.ID, ShouldNotEqual, uuid.Nil)
			})

			Convey("Then GetAllRoutingProfiles includes the created routing-profile", func() {
				rpGetAll, err := GetAllRoutingProfiles(db)
				So(err, ShouldBeNil)
				So(rpGetAll, ShouldHaveLength, 1)

				rpGetAll[0].CreatedAt = rpGetAll[0].CreatedAt.UTC().Truncate(time.Millisecond)
				rpGetAll[0].UpdatedAt = rpGetAll[0].UpdatedAt.UTC().Truncate(time.Millisecond)
				So(rpGetAll[0], ShouldResemble, rp)
			})

			Convey("Then UpdateRoutingProfile updates the routing-profile", func() {
				rp.ASID = "new-application-server:1234"
				rp.CACert = "CACERT2"
				rp.TLSCert = "TLSCERT2"
				rp.TLSKey = "TLSKEY2"
				So(UpdateRoutingProfile(db, &rp), ShouldBeNil)
				rp.UpdatedAt = rp.UpdatedAt.UTC().Truncate(time.Millisecond)

				rpGet, err := GetRoutingProfile(db, rp.ID)
				So(err, ShouldBeNil)

				rpGet.CreatedAt = rpGet.CreatedAt.UTC().Truncate(time.Millisecond)
				rpGet.UpdatedAt = rpGet.UpdatedAt.UTC().Truncate(time.Millisecond)
				So(rpGet, ShouldResemble, rp)
			})

			Convey("Then DeleteRoutingProfile deletes the routing-profile", func() {
				So(DeleteRoutingProfile(db, rp.ID), ShouldBeNil)
				So(DeleteRoutingProfile(db, rp.ID), ShouldEqual, ErrDoesNotExist)
			})
		})
	})
}
