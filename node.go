package loraserver

import (
	"database/sql/driver"
	"errors"
	"fmt"

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
	DevEUI        lorawan.EUI64     `db:"dev_eui"`
	AppEUI        lorawan.EUI64     `db:"app_eui"`
	AppKey        lorawan.AES128Key `db:"app_key"`
	UsedDevNonces DevNonceList      `db:"used_dev_nonces"`
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
	return err
}

// UpdateNode updates the given Node.
func UpdateNode(db *sqlx.DB, n Node) error {
	_, err := db.Exec("update node set app_eui = $1, app_key = $2, used_dev_nonces = $3 where dev_eui = $4",
		n.AppEUI[:],
		n.AppKey[:],
		n.UsedDevNonces,
		n.DevEUI[:],
	)
	return err
}

// GetNode returns the Node for the given DevEUI.
func GetNode(db *sqlx.DB, devEUI lorawan.EUI64) (Node, error) {
	var node Node
	return node, db.Get(&node, "select * from node where dev_eui = $1", devEUI[:])
}

// NodeABP contains the Activation By Personalization of a node (if any).
// Note that the FCntUp and FCntDown are the initial values as how the
// node needs to be activated. The real counting happens in NodeSession
// (for performance reasons).
type NodeABP struct {
	DevEUI   lorawan.EUI64
	DevAddr  lorawan.DevAddr
	AppSKey  lorawan.AES128Key
	NwkSKey  lorawan.AES128Key
	FCntUp   uint32 // the next expected value
	FCntDown uint32 // the next expected value
}

// CreateNodeABP creates the given NodeABP.
func CreateNodeABP(db *sqlx.DB, n NodeABP) error {
	_, err := db.Exec("insert into node_abp (dev_eui, dev_addr, app_s_key, nwk_s_key, fcnt_up, fcnt_down) values ($1, $2, $3, $4, $5, $6)",
		n.DevEUI[:],
		n.DevAddr[:],
		n.AppSKey[:],
		n.NwkSKey[:],
		n.FCntUp,
		n.FCntDown,
	)
	return err
}
