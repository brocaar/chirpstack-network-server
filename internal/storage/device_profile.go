package storage

import (
	"time"

	"github.com/lib/pq"

	"github.com/jmoiron/sqlx"
	uuid "github.com/satori/go.uuid"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/lorawan/backend"
)

// DeviceProfile defines the backend.DeviceProfile with some extra meta-data
type DeviceProfile struct {
	CreatedBy string    `db:"created_by"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
	backend.DeviceProfile
}

// CreateDeviceProfile creates the given device-profile.
func CreateDeviceProfile(db *sqlx.DB, dp *DeviceProfile) error {
	now := time.Now()
	dp.DeviceProfile.DeviceProfileID = uuid.NewV4().String()
	dp.CreatedAt = now
	dp.UpdatedAt = now

	_, err := db.Exec(`
		insert into device_profile (
			created_by,
			created_at,
			updated_at,

			device_profile_id,
			supports_class_b,
			class_b_timeout,
			ping_slot_period,
			ping_slot_dr,
			ping_slot_freq,
			supports_class_c,
			class_c_timeout,
			mac_version,
			reg_params_revision,
			rx_delay_1,
			rx_dr_offset_1,
			rx_data_rate_2,
			rx_freq_2,
			factory_preset_freqs,
			max_eirp,
			max_duty_cycle,
			supports_join,
			rf_region,
			supports_32bit_fcnt
		) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23)`,
		dp.CreatedBy,
		dp.CreatedAt,
		dp.UpdatedAt,
		dp.DeviceProfile.DeviceProfileID,
		dp.DeviceProfile.SupportsClassB,
		dp.DeviceProfile.ClassBTimeout,
		dp.DeviceProfile.PingSlotPeriod,
		dp.DeviceProfile.PingSlotDR,
		dp.DeviceProfile.PingSlotFreq,
		dp.DeviceProfile.SupportsClassC,
		dp.DeviceProfile.ClassCTimeout,
		dp.DeviceProfile.MACVersion,
		dp.DeviceProfile.RegParamsRevision,
		dp.DeviceProfile.RXDelay1,
		dp.DeviceProfile.RXDROffset1,
		dp.DeviceProfile.RXDataRate2,
		dp.DeviceProfile.RXFreq2,
		pq.Array(dp.DeviceProfile.FactoryPresetFreqs),
		dp.DeviceProfile.MaxEIRP,
		dp.DeviceProfile.MaxDutyCycle,
		dp.DeviceProfile.SupportsJoin,
		dp.DeviceProfile.RFRegion,
		dp.DeviceProfile.Supports32bitFCnt,
	)
	if err != nil {
		return handlePSQLError(err, "insert error")
	}

	log.WithFields(log.Fields{
		"created_by":        dp.CreatedBy,
		"device_profile_id": dp.DeviceProfile.DeviceProfileID,
	}).Info("device-profile created")

	return nil
}

// GetDeviceProfile returns the device-profile matching the given id.
func GetDeviceProfile(db *sqlx.DB, id string) (DeviceProfile, error) {
	var dp DeviceProfile

	row := db.QueryRow(`
		select
			created_by,
			created_at,
			updated_at,

			device_profile_id,
			supports_class_b,
			class_b_timeout,
			ping_slot_period,
			ping_slot_dr,
			ping_slot_freq,
			supports_class_c,
			class_c_timeout,
			mac_version,
			reg_params_revision,
			rx_delay_1,
			rx_dr_offset_1,
			rx_data_rate_2,
			rx_freq_2,
			factory_preset_freqs,
			max_eirp,
			max_duty_cycle,
			supports_join,
			rf_region,
			supports_32bit_fcnt
		from device_profile
		where
			device_profile_id = $1
		`, id)

	err := row.Scan(
		&dp.CreatedBy,
		&dp.CreatedAt,
		&dp.UpdatedAt,
		&dp.DeviceProfile.DeviceProfileID,
		&dp.DeviceProfile.SupportsClassB,
		&dp.DeviceProfile.ClassBTimeout,
		&dp.DeviceProfile.PingSlotPeriod,
		&dp.DeviceProfile.PingSlotDR,
		&dp.DeviceProfile.PingSlotFreq,
		&dp.DeviceProfile.SupportsClassC,
		&dp.DeviceProfile.ClassCTimeout,
		&dp.DeviceProfile.MACVersion,
		&dp.DeviceProfile.RegParamsRevision,
		&dp.DeviceProfile.RXDelay1,
		&dp.DeviceProfile.RXDROffset1,
		&dp.DeviceProfile.RXDataRate2,
		&dp.DeviceProfile.RXFreq2,
		pq.Array(&dp.DeviceProfile.FactoryPresetFreqs),
		&dp.DeviceProfile.MaxEIRP,
		&dp.DeviceProfile.MaxDutyCycle,
		&dp.DeviceProfile.SupportsJoin,
		&dp.DeviceProfile.RFRegion,
		&dp.DeviceProfile.Supports32bitFCnt,
	)
	if err != nil {
		return dp, handlePSQLError(err, "select error")
	}

	return dp, nil
}

// UpdateDeviceProfile updates the given device-profile.
func UpdateDeviceProfile(db *sqlx.DB, dp *DeviceProfile) error {
	dp.UpdatedAt = time.Now()

	res, err := db.Exec(`
		update device_profile set
			updated_at = $2,

			supports_class_b = $3,
			class_b_timeout = $4,
			ping_slot_period = $5,
			ping_slot_dr = $6,
			ping_slot_freq = $7,
			supports_class_c = $8,
			class_c_timeout = $9,
			mac_version = $10,
			reg_params_revision = $11,
			rx_delay_1 = $12,
			rx_dr_offset_1 = $13,
			rx_data_rate_2 = $14,
			rx_freq_2 = $15,
			factory_preset_freqs = $16,
			max_eirp = $17,
			max_duty_cycle = $18,
			supports_join = $19,
			rf_region = $20,
			supports_32bit_fcnt = $21
		where
			device_profile_id = $1`,
		dp.DeviceProfile.DeviceProfileID,
		dp.UpdatedAt,
		dp.DeviceProfile.SupportsClassB,
		dp.DeviceProfile.ClassBTimeout,
		dp.DeviceProfile.PingSlotPeriod,
		dp.DeviceProfile.PingSlotDR,
		dp.DeviceProfile.PingSlotFreq,
		dp.DeviceProfile.SupportsClassC,
		dp.DeviceProfile.ClassCTimeout,
		dp.DeviceProfile.MACVersion,
		dp.DeviceProfile.RegParamsRevision,
		dp.DeviceProfile.RXDelay1,
		dp.DeviceProfile.RXDROffset1,
		dp.DeviceProfile.RXDataRate2,
		dp.DeviceProfile.RXFreq2,
		pq.Array(dp.DeviceProfile.FactoryPresetFreqs),
		dp.DeviceProfile.MaxEIRP,
		dp.DeviceProfile.MaxDutyCycle,
		dp.DeviceProfile.SupportsJoin,
		dp.DeviceProfile.RFRegion,
		dp.DeviceProfile.Supports32bitFCnt,
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

	log.WithField("device_profile_id", dp.DeviceProfile.DeviceProfileID).Info("device-profile updated")
	return nil
}

// DeleteDeviceProfile deletes the device-profile matching the given id.
func DeleteDeviceProfile(db *sqlx.DB, id string) error {
	res, err := db.Exec("delete from device_profile where device_profile_id = $1", id)
	if err != nil {
		return handlePSQLError(err, "delete error")
	}

	ra, err := res.RowsAffected()
	if err != nil {
		return handlePSQLError(err, "get rows affacted error")
	}
	if ra == 0 {
		return ErrDoesNotExist
	}
	log.WithField("device_profile_id", id).Info("device-profile deleted")
	return nil
}
