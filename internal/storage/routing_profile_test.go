package storage

import (
	"context"
	"testing"
	"time"

	"github.com/gofrs/uuid"
	. "github.com/smartystreets/goconvey/convey"

	"github.com/brocaar/loraserver/internal/test"
)

func TestRoutingProfile(t *testing.T) {
	conf := test.GetConfig()
	if err := Setup(conf); err != nil {
		t.Fatal(err)
	}

	Convey("Given a clean database", t, func() {
		test.MustResetDB(DB().DB)

		Convey("When creating a routing-profile", func() {
			rp := RoutingProfile{
				ASID:    "application-server:1234",
				CACert:  "CACERT",
				TLSCert: "TLSCERT",
				TLSKey:  "TLSKEY",
			}
			So(CreateRoutingProfile(context.Background(), DB(), &rp), ShouldBeNil)
			rp.CreatedAt = rp.CreatedAt.UTC().Truncate(time.Millisecond)
			rp.UpdatedAt = rp.UpdatedAt.UTC().Truncate(time.Millisecond)

			Convey("Then GetRoutingProfile returns the expected routing-profile", func() {
				rpGet, err := GetRoutingProfile(context.Background(), DB(), rp.ID)
				So(err, ShouldBeNil)

				rpGet.CreatedAt = rpGet.CreatedAt.UTC().Truncate(time.Millisecond)
				rpGet.UpdatedAt = rpGet.UpdatedAt.UTC().Truncate(time.Millisecond)
				So(rpGet, ShouldResemble, rp)
				So(rpGet.ID, ShouldNotEqual, uuid.Nil)
			})

			Convey("Then GetAllRoutingProfiles includes the created routing-profile", func() {
				rpGetAll, err := GetAllRoutingProfiles(context.Background(), DB())
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
				So(UpdateRoutingProfile(context.Background(), DB(), &rp), ShouldBeNil)
				rp.UpdatedAt = rp.UpdatedAt.UTC().Truncate(time.Millisecond)

				rpGet, err := GetRoutingProfile(context.Background(), DB(), rp.ID)
				So(err, ShouldBeNil)

				rpGet.CreatedAt = rpGet.CreatedAt.UTC().Truncate(time.Millisecond)
				rpGet.UpdatedAt = rpGet.UpdatedAt.UTC().Truncate(time.Millisecond)
				So(rpGet, ShouldResemble, rp)
			})

			Convey("Then DeleteRoutingProfile deletes the routing-profile", func() {
				So(DeleteRoutingProfile(context.Background(), DB(), rp.ID), ShouldBeNil)
				So(DeleteRoutingProfile(context.Background(), DB(), rp.ID), ShouldEqual, ErrDoesNotExist)
			})
		})
	})
}
