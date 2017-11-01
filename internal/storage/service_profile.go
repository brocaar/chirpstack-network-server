package storage

import (
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/satori/go.uuid"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/lorawan/backend"
)

// ServiceProfile defines the backend.ServiceProfile with some extra meta-data.
type ServiceProfile struct {
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
	backend.ServiceProfile
}

// CreateServiceProfile creates the given service-profile.
func CreateServiceProfile(db *sqlx.DB, sp *ServiceProfile) error {
	now := time.Now()
	if sp.ServiceProfile.ServiceProfileID == "" {
		sp.ServiceProfile.ServiceProfileID = uuid.NewV4().String()
	}
	sp.CreatedAt = now
	sp.UpdatedAt = now

	_, err := db.Exec(`
		insert into service_profile (
			created_at,
			updated_at,

			service_profile_id,
			ul_rate,
			ul_bucket_size,
			ul_rate_policy,
			dl_rate,
			dl_bucket_size,
			dl_rate_policy,
			add_gw_metadata,
			dev_status_req_freq,
			report_dev_status_battery,
			report_dev_status_margin,
			dr_min,
			dr_max,
			channel_mask,
			pr_allowed,
			hr_allowed,
			ra_allowed,
			nwk_geo_loc,
			target_per,
			min_gw_diversity
		) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22)`,
		sp.CreatedAt,
		sp.UpdatedAt,
		sp.ServiceProfile.ServiceProfileID,
		sp.ServiceProfile.ULRate,
		sp.ServiceProfile.ULBucketSize,
		sp.ServiceProfile.ULRatePolicy,
		sp.ServiceProfile.DLRate,
		sp.ServiceProfile.DLBucketSize,
		sp.ServiceProfile.DLRatePolicy,
		sp.ServiceProfile.AddGWMetadata,
		sp.ServiceProfile.DevStatusReqFreq,
		sp.ServiceProfile.ReportDevStatusBattery,
		sp.ServiceProfile.ReportDevStatusMargin,
		sp.ServiceProfile.DRMin,
		sp.ServiceProfile.DRMax,
		sp.ServiceProfile.ChannelMask,
		sp.ServiceProfile.PRAllowed,
		sp.ServiceProfile.HRAllowed,
		sp.ServiceProfile.RAAllowed,
		sp.ServiceProfile.NwkGeoLoc,
		sp.ServiceProfile.TargetPER,
		sp.ServiceProfile.MinGWDiversity,
	)
	if err != nil {
		return handlePSQLError(err, "insert error")
	}

	log.WithFields(log.Fields{
		"service_profile_id": sp.ServiceProfile.ServiceProfileID,
	}).Info("service-profile created")

	return nil
}

// GetServiceProfile returns the service-profile matching the given id.
func GetServiceProfile(db *sqlx.DB, id string) (ServiceProfile, error) {
	var sp ServiceProfile
	err := db.Get(&sp, "select * from service_profile where service_profile_id = $1", id)
	if err != nil {
		return sp, handlePSQLError(err, "select error")
	}

	return sp, nil
}

// UpdateServiceProfile updates the given service-profile.
func UpdateServiceProfile(db *sqlx.DB, sp *ServiceProfile) error {
	sp.UpdatedAt = time.Now()

	res, err := db.Exec(`
		update service_profile set
			updated_at = $2,

			ul_rate = $3,
			ul_bucket_size = $4,
			ul_rate_policy = $5,
			dl_rate = $6,
			dl_bucket_size = $7,
			dl_rate_policy = $8,
			add_gw_metadata = $9,
			dev_status_req_freq = $10,
			report_dev_status_battery = $11,
			report_dev_status_margin = $12,
			dr_min = $13,
			dr_max = $14,
			channel_mask = $15,
			pr_allowed = $16,
			hr_allowed = $17,
			ra_allowed = $18,
			nwk_geo_loc = $19,
			target_per = $20,
			min_gw_diversity = $21
		where
			service_profile_id = $1`,
		sp.ServiceProfile.ServiceProfileID,
		sp.UpdatedAt,
		sp.ServiceProfile.ULRate,
		sp.ServiceProfile.ULBucketSize,
		sp.ServiceProfile.ULRatePolicy,
		sp.ServiceProfile.DLRate,
		sp.ServiceProfile.DLBucketSize,
		sp.ServiceProfile.DLRatePolicy,
		sp.ServiceProfile.AddGWMetadata,
		sp.ServiceProfile.DevStatusReqFreq,
		sp.ServiceProfile.ReportDevStatusBattery,
		sp.ServiceProfile.ReportDevStatusMargin,
		sp.ServiceProfile.DRMin,
		sp.ServiceProfile.DRMax,
		sp.ServiceProfile.ChannelMask,
		sp.ServiceProfile.PRAllowed,
		sp.ServiceProfile.HRAllowed,
		sp.ServiceProfile.RAAllowed,
		sp.ServiceProfile.NwkGeoLoc,
		sp.ServiceProfile.TargetPER,
		sp.ServiceProfile.MinGWDiversity,
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

	log.WithField("service_profile_id", sp.ServiceProfile.ServiceProfileID).Info("service-profile updated")
	return nil
}

// DeleteServiceProfile deletes the service-profile matching the given id.
func DeleteServiceProfile(db *sqlx.DB, id string) error {
	res, err := db.Exec("delete from service_profile where service_profile_id = $1", id)
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

	log.WithField("service_profile_id", id).Info("service-profile deleted")
	return nil
}
