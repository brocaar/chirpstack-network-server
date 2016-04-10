package models

import (
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/band"
)

// NodeSession contains the informatio of a node-session (an activated node).
type NodeSession struct {
	DevAddr  lorawan.DevAddr   `db:"dev_addr" json:"devAddr"`
	AppEUI   lorawan.EUI64     `db:"app_eui" json:"appEUI"`
	DevEUI   lorawan.EUI64     `db:"dev_eui" json:"devEUI"`
	AppSKey  lorawan.AES128Key `db:"app_s_key" json:"appSKey"`
	NwkSKey  lorawan.AES128Key `db:"nwk_s_key" json:"nwkSKey"`
	FCntUp   uint32            `db:"fcnt_up" json:"fCntUp"`
	FCntDown uint32            `db:"fcnt_down" json:"fCntDown"`
}

// ValidateAndGetFullFCntUp validates if the given fCntUp is valid
// and returns the full 32 bit frame-counter.
// Note that the LoRaWAN packet only contains the 16 LSB, so in order
// to validate the MIC, the full 32 bit frame-counter needs to be set.
// After a succesful validation of the FCntUp and the MIC, don't forget
// to synchronize the Node FCntUp with the packet FCnt.
func (n NodeSession) ValidateAndGetFullFCntUp(fCntUp uint32) (uint32, bool) {
	// we need to compare the difference of the 16 LSB
	gap := uint32(uint16(fCntUp) - uint16(n.FCntUp%65536))
	if gap < band.MaxFCntGap {
		return n.FCntUp + gap, true
	}
	return 0, false
}
