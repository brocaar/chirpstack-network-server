package models

import (
	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/backend"
	"github.com/gofrs/uuid"
)

// RXPacket contains a received PHYPayload together with its RX metadata.
type RXPacket struct {
	// DR contains the data-rate index of the uplink.
	DR int

	// PHYPayload holds the uplink PHYPayload object.
	PHYPayload lorawan.PHYPayload

	// TXInfo holds the TX meta-data struct.
	TXInfo *gw.UplinkTXInfo

	// RXInfoSet holds all the RX meta-data elements of the receiving gateways.
	RXInfoSet []*gw.UplinkRXInfo

	// GatewayIsPrivate holds if the Gateway ID is private.
	GatewayIsPrivate map[lorawan.EUI64]bool

	// GatewayServiceProfile holds the Gateway ID to service-profile ID mapping.
	GatewayServiceProfile map[lorawan.EUI64]uuid.UUID

	// RoamingMetaData holds the meta-data in case of a roaming device.
	RoamingMetaData *RoamingMetaData
}

// RoamingMetaData holds the Backend Interfaces roaming meta-data.
type RoamingMetaData struct {
	BasePayload backend.BasePayload
	ULMetaData  backend.ULMetaData
}
