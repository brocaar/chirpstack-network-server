package models

import (
	"database/sql/driver"
	"errors"
	"fmt"

	"github.com/brocaar/lorawan"
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

	RXDelay       uint8  `db:"rx_delay" json:"rxDelay"`
	RX1DROffset   uint8  `db:"rx1_dr_offset" json:"rx1DROffset"`
	ChannelListID *int64 `db:"channel_list_id" json:"channelListID"`
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
