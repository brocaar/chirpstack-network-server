package models

import "github.com/brocaar/lorawan"

// NodeSession contains the informatio of a node-session (an activated node).
type NodeSession struct {
	DevAddr  lorawan.DevAddr   `db:"dev_addr" json:"devAddr"`
	AppEUI   lorawan.EUI64     `db:"app_eui" json:"appEUI"`
	DevEUI   lorawan.EUI64     `db:"dev_eui" json:"devEUI"`
	AppSKey  lorawan.AES128Key `db:"app_s_key" json:"appSKey"`
	NwkSKey  lorawan.AES128Key `db:"nwk_s_key" json:"nwkSKey"`
	FCntUp   uint32            `db:"fcnt_up" json:"fCntUp"`
	FCntDown uint32            `db:"fcnt_down" json:"fCntDown"`
	RXDelay  uint8             `db:"rx_delay" json:"rxDelay"`
}
