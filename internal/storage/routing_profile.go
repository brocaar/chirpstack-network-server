package storage

import (
	"context"
	"time"

	"github.com/brocaar/chirpstack-api/go/v3/as"
	"github.com/brocaar/chirpstack-network-server/internal/backend/applicationserver"
	"github.com/brocaar/chirpstack-network-server/internal/logging"
	"github.com/gofrs/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// RoutingProfile defines the backend.RoutingProfile with some extra meta-data.
type RoutingProfile struct {
	ID        uuid.UUID `json:"RoutingProfileID" db:"routing_profile_id"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
	ASID      string    `json:"AS-ID" db:"as_id"` // Value can be IP address, DNS name, etc.
	CACert    string    `db:"ca_cert"`
	TLSCert   string    `db:"tls_cert"`
	TLSKey    string    `db:"tls_key"`
}

// GetApplicationServerClient returns the application-server client.
func (rp RoutingProfile) GetApplicationServerClient() (as.ApplicationServerServiceClient, error) {
	asClient, err := applicationserver.Pool().Get(
		rp.ASID,
		[]byte(rp.CACert),
		[]byte(rp.TLSCert),
		[]byte(rp.TLSKey),
	)
	if err != nil {
		return nil, errors.Wrap(err, "get application-server client error")
	}

	return asClient, nil
}

// CreateRoutingProfile creates the given routing-profile.
func CreateRoutingProfile(ctx context.Context, db sqlx.Execer, rp *RoutingProfile) error {
	now := time.Now()

	if rp.ID == uuid.Nil {
		var err error
		rp.ID, err = uuid.NewV4()
		if err != nil {
			return errors.Wrap(err, "new uuid v4 error")
		}
	}

	rp.CreatedAt = now
	rp.UpdatedAt = now

	_, err := db.Exec(`
		insert into routing_profile (
			created_at,
			updated_at,

			routing_profile_id,
			as_id,
			ca_cert,
			tls_cert,
			tls_key
		) values ($1, $2, $3, $4, $5, $6, $7)`,
		rp.CreatedAt,
		rp.UpdatedAt,
		rp.ID,
		rp.ASID,
		rp.CACert,
		rp.TLSCert,
		rp.TLSKey,
	)
	if err != nil {
		return handlePSQLError(err, "insert error")
	}

	log.WithFields(log.Fields{
		"id":     rp.ID,
		"ctx_id": ctx.Value(logging.ContextIDKey),
	}).Info("routing-profile created")

	return nil
}

// GetRoutingProfile returns the routing-profile matching the given id.
func GetRoutingProfile(ctx context.Context, db sqlx.Queryer, id uuid.UUID) (RoutingProfile, error) {
	var rp RoutingProfile
	err := sqlx.Get(db, &rp, "select * from routing_profile where routing_profile_id = $1", id)
	if err != nil {
		return rp, handlePSQLError(err, "select error")
	}

	return rp, nil
}

// UpdateRoutingProfile updates the given routing-profile.
func UpdateRoutingProfile(ctx context.Context, db sqlx.Execer, rp *RoutingProfile) error {
	rp.UpdatedAt = time.Now()
	res, err := db.Exec(`
		update routing_profile set
			updated_at = $2,
			as_id = $3,
			ca_cert = $4,
			tls_cert = $5,
			tls_key = $6
		where
			routing_profile_id = $1`,
		rp.ID,
		rp.UpdatedAt,
		rp.ASID,
		rp.CACert,
		rp.TLSCert,
		rp.TLSKey,
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

	log.WithFields(log.Fields{
		"id":     rp.ID,
		"ctx_id": ctx.Value(logging.ContextIDKey),
	}).Info("routing-profile updated")
	return nil
}

// DeleteRoutingProfile deletes the routing-profile matching the given id.
func DeleteRoutingProfile(ctx context.Context, db sqlx.Execer, id uuid.UUID) error {
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

	log.WithFields(log.Fields{
		"id":     id,
		"ctx_id": ctx.Value(logging.ContextIDKey),
	}).Info("routing-profile deleted")
	return nil
}

// GetAllRoutingProfiles returns all the available routing-profiles.
func GetAllRoutingProfiles(ctx context.Context, db sqlx.Queryer) ([]RoutingProfile, error) {
	var rps []RoutingProfile
	err := sqlx.Select(db, &rps, "select * from routing_profile")
	if err != nil {
		return nil, handlePSQLError(err, "select error")
	}
	return rps, nil
}
