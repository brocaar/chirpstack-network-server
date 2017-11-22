package storage

import (
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/satori/go.uuid"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/lorawan/backend"
)

// RoutingProfile defines the backend.RoutingProfile with some extra meta-data.
type RoutingProfile struct {
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
	backend.RoutingProfile
}

// CreateRoutingProfile creates the given routing-profile.
func CreateRoutingProfile(db sqlx.Execer, rp *RoutingProfile) error {
	now := time.Now()
	if rp.RoutingProfile.RoutingProfileID == "" {
		rp.RoutingProfile.RoutingProfileID = uuid.NewV4().String()
	}
	rp.CreatedAt = now
	rp.UpdatedAt = now

	_, err := db.Exec(`
		insert into routing_profile (
			created_at,
			updated_at,

			routing_profile_id,
			as_id
		) values ($1, $2, $3, $4)`,
		rp.CreatedAt,
		rp.UpdatedAt,
		rp.RoutingProfile.RoutingProfileID,
		rp.RoutingProfile.ASID,
	)
	if err != nil {
		return handlePSQLError(err, "insert error")
	}

	log.WithFields(log.Fields{
		"routing_profile_id": rp.RoutingProfile.RoutingProfileID,
	}).Info("routing-profile created")

	return nil
}

// GetRoutingProfile returns the routing-profile matching the given id.
func GetRoutingProfile(db sqlx.Queryer, id string) (RoutingProfile, error) {
	var rp RoutingProfile
	err := sqlx.Get(db, &rp, "select * from routing_profile where routing_profile_id = $1", id)
	if err != nil {
		return rp, handlePSQLError(err, "select error")
	}

	return rp, nil
}

// UpdateRoutingProfile updates the given routing-profile.
func UpdateRoutingProfile(db sqlx.Execer, rp *RoutingProfile) error {
	rp.UpdatedAt = time.Now()
	res, err := db.Exec(`
		update routing_profile set
			updated_at = $2,
			as_id = $3
		where
			routing_profile_id = $1`,
		rp.RoutingProfile.RoutingProfileID,
		rp.UpdatedAt,
		rp.RoutingProfile.ASID,
	)
	if err != nil {
		return handlePSQLError(err, "update error")
	}
	ra, err := res.RowsAffected()
	if err != nil {
		return handlePSQLError(err, "get rows affected error")
	}
	if ra == 0 {
		return ErrDoesNotExist
	}

	log.WithField("routing_profile_id", rp.RoutingProfile.RoutingProfileID).Info("routing-profile updated")
	return nil
}

// DeleteRoutingProfile deletes the routing-profile matching the given id.
func DeleteRoutingProfile(db sqlx.Execer, id string) error {
	res, err := db.Exec("delete from routing_profile where routing_profile_id = $1", id)
	if err != nil {
		return handlePSQLError(err, "delete error")
	}

	ra, err := res.RowsAffected()
	if err != nil {
		return handlePSQLError(err, "get rows affected error")
	}
	if ra == 0 {
		return ErrDoesNotExist
	}

	log.WithField("routing_profile_id", id).Info("routing-profile deleted")
	return nil
}

// GetAllRoutingProfiles returns all the available routing-profiles.
func GetAllRoutingProfiles(db sqlx.Queryer) ([]RoutingProfile, error) {
	var rps []RoutingProfile
	err := sqlx.Select(db, &rps, "select * from routing_profile")
	if err != nil {
		return nil, handlePSQLError(err, "select error")
	}
	return rps, nil
}
