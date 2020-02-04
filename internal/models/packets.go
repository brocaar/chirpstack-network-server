package models

import (
	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/brocaar/lorawan"
)

// RXPacket contains a received PHYPayload together with its RX metadata.
type RXPacket struct {
	DR         int
	PHYPayload lorawan.PHYPayload
	TXInfo     *gw.UplinkTXInfo
	RXInfoSet  []*gw.UplinkRXInfo
}
