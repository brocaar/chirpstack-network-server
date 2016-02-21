package loraserver

import "github.com/brocaar/lorawan"

// Node contains the information of a node.
type Node struct {
	DevEUI        lorawan.EUI64
	AppEUI        lorawan.EUI64
	AppKey        lorawan.AES128Key
	UsedDevNonces [][2]byte
}
