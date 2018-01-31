package node

import (
	"time"

	"github.com/Frankz/lorawan"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

// FrameLog contains the log of a node uplink or downlink frame.
type FrameLog struct {
	ID         int64         `db:"id"`
	CreatedAt  time.Time     `db:"created_at"`
	DevEUI     lorawan.EUI64 `db:"dev_eui"`
	RXInfoSet  *[]byte       `db:"rx_info_set"`
	TXInfo     *[]byte       `db:"tx_info"`
	PHYPayload []byte        `db:"phy_payload"`
}

// CreateFrameLog creates a log entry for the given data.
func CreateFrameLog(db sqlx.Execer, fl *FrameLog) error {
	now := time.Now()
	_, err := db.Exec(`
		insert into frame_log (
			created_at,
			dev_eui,
			rx_info_set,
			tx_info,
			phy_payload
		) values ($1, $2, $3, $4, $5)`,
		now,
		fl.DevEUI[:],
		fl.RXInfoSet,
		fl.TXInfo,
		fl.PHYPayload,
	)
	if err != nil {
		return errors.Wrap(err, "insert error")
	}
	fl.CreatedAt = now
	return nil
}

// GetFrameLogCountForDevEUI returns the total number of log items for the
// given DevEUI.
func GetFrameLogCountForDevEUI(db sqlx.Queryer, devEUI lorawan.EUI64) (int, error) {
	var count int
	err := sqlx.Get(db, &count, "select count(*) from frame_log where dev_eui = $1", devEUI[:])
	if err != nil {
		return 0, errors.Wrap(err, "select error")
	}
	return count, nil
}

// GetFrameLogsForDevEUI returns the frame logs sorted by timestamp (decending)
// for the given DevEUI and the given limit and offset.
func GetFrameLogsForDevEUI(db sqlx.Queryer, devEUI lorawan.EUI64, limit, offset int) ([]FrameLog, error) {
	var logs []FrameLog
	err := sqlx.Select(db, &logs, `
		select *
		from frame_log
		where dev_eui = $1
		order by created_at desc
		limit $2
		offset $3`,
		devEUI[:],
		limit,
		offset,
	)
	if err != nil {
		return nil, errors.Wrap(err, "select error")
	}
	return logs, nil
}
