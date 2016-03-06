package loraserver

import (
	"database/sql/driver"
	"errors"
	"fmt"

	log "github.com/Sirupsen/logrus"
	"github.com/brocaar/lorawan"
	"github.com/jmoiron/sqlx"
)

// UsedDevNonceCount is the number of used dev-nonces to track.
const UsedDevNonceCount = 10

// DevNonceList represents a list of dev nonces
type DevNonceList [][2]byte

// Scan implements the sql.Scanner interface.
func (l *DevNonceList) Scan(src interface{}) error {
	if src == nil {
		*l = make([][2]byte, 0)
		return nil
	}

	b, ok := src.([]byte)
	if !ok {
		return fmt.Errorf("src must be of type []byte, got: %T", src)
	}
	if len(b)%2 != 0 {
		return errors.New("the length of src must be a multiple of 2")
	}
	for i := 0; i < len(b); i += 2 {
		*l = append(*l, [2]byte{b[i], b[i+1]})
	}
	return nil
}

// Value implements the driver.Valuer interface.
func (l DevNonceList) Value() (driver.Value, error) {
	b := make([]byte, 0, len(l)/2)
	for _, n := range l {
		b = append(b, n[:]...)
	}
	return b, nil
}

// Node contains the information of a node.
type Node struct {
	DevEUI        lorawan.EUI64     `db:"dev_eui" json:"devEUI"`
	AppEUI        lorawan.EUI64     `db:"app_eui" json:"appEUI"`
	AppKey        lorawan.AES128Key `db:"app_key" json:"appKey"`
	UsedDevNonces DevNonceList      `db:"used_dev_nonces" json:"usedDevNonces"`
}

// ValidateDevNonce returns if the given dev-nonce is valid.
// When valid, it will be added to UsedDevNonces. This does
// not update the Node in the database!
func (n *Node) ValidateDevNonce(nonce [2]byte) bool {
	for _, used := range n.UsedDevNonces {
		if nonce == used {
			return false
		}
	}
	n.UsedDevNonces = append(n.UsedDevNonces, nonce)
	if len(n.UsedDevNonces) > UsedDevNonceCount {
		n.UsedDevNonces = n.UsedDevNonces[len(n.UsedDevNonces)-UsedDevNonceCount:]
	}

	return true
}

// CreateNode creates the given Node.
func CreateNode(db *sqlx.DB, n Node) error {
	_, err := db.Exec("insert into node (dev_eui, app_eui, app_key) values ($1, $2, $3)",
		n.DevEUI[:],
		n.AppEUI[:],
		n.AppKey[:],
	)
	if err == nil {
		log.WithField("dev_eui", n.DevEUI).Info("node created")
	}
	return err
}

// UpdateNode updates the given Node.
func UpdateNode(db *sqlx.DB, n Node) error {
	res, err := db.Exec("update node set app_eui = $1, app_key = $2, used_dev_nonces = $3 where dev_eui = $4",
		n.AppEUI[:],
		n.AppKey[:],
		n.UsedDevNonces,
		n.DevEUI[:],
	)
	if err != nil {
		return err
	}
	ra, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if ra == 0 {
		return errors.New("DevEUI did not match any rows")
	}
	log.WithField("dev_eui", n.DevEUI).Info("node updated")
	return nil
}

// DeleteNode deletes the Node matching the given DevEUI.
func DeleteNode(db *sqlx.DB, devEUI lorawan.EUI64) error {
	res, err := db.Exec("delete from node where dev_eui = $1",
		devEUI[:],
	)
	if err != nil {
		return err
	}
	ra, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if ra == 0 {
		return errors.New("DevEUI did not match any rows")
	}
	log.WithField("dev_eui", devEUI).Info("node deleted")
	return nil
}

// GetNode returns the Node for the given DevEUI.
func GetNode(db *sqlx.DB, devEUI lorawan.EUI64) (Node, error) {
	var node Node
	return node, db.Get(&node, "select * from node where dev_eui = $1", devEUI[:])
}
