package loraserver

import (
	"github.com/brocaar/lorawan"
	"github.com/jmoiron/sqlx"
)

// UsedDevNonceCount is the number of used dev-nonces to track.
const UsedDevNonceCount = 10

// Node contains the information of a node.
type Node struct {
	DevEUI        lorawan.EUI64
	AppEUI        lorawan.EUI64
	AppKey        lorawan.AES128Key
	UsedDevNonces [][2]byte
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
