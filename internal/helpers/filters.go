package helpers

import (
	"github.com/gofrs/uuid"
	"github.com/pkg/errors"

	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/brocaar/chirpstack-network-server/internal/config"
	"github.com/brocaar/chirpstack-network-server/internal/models"
)

var (
	// ErrNoElements is returned when no RxInfo elements are matching the filter
	// criteria.
	ErrNoElements = errors.New("no elements to return")
)

// FilterRxInfoByPublicOnly filters the RxInfo elements on public gateways.
func FilterRxInfoByPublicOnly(rxPacket *models.RXPacket) error {
	// In case the ForceGwsPrivate is set, return ErrNoElements as none of the
	// gateways are public.
	conf := config.Get()
	if conf.NetworkServer.Gateway.ForceGwsPrivate {
		return ErrNoElements
	}

	var rxInfoSet []*gw.UplinkRXInfo

	for i := range rxPacket.RXInfoSet {
		rxInfo := rxPacket.RXInfoSet[i]
		id := GetGatewayID(rxInfo)

		if !rxPacket.GatewayIsPrivate[id] {
			rxInfoSet = append(rxInfoSet, rxInfo)
		}
	}

	if len(rxInfoSet) == 0 {
		return ErrNoElements
	}

	rxPacket.RXInfoSet = rxInfoSet
	return nil
}

// FilterRxInfoByServiceProfileID filters the RxInfo elements on public gateways
// and gateways matching the given ServiceProfileID.
func FilterRxInfoByServiceProfileID(serviceProfileID uuid.UUID, rxPacket *models.RXPacket) error {
	var rxInfoSet []*gw.UplinkRXInfo
	conf := config.Get()

	for i := range rxPacket.RXInfoSet {
		rxInfo := rxPacket.RXInfoSet[i]
		id := GetGatewayID(rxInfo)

		if !(rxPacket.GatewayIsPrivate[id] || conf.NetworkServer.Gateway.ForceGwsPrivate) || rxPacket.GatewayServiceProfile[id] == serviceProfileID {
			rxInfoSet = append(rxInfoSet, rxInfo)
		}
	}

	if len(rxInfoSet) == 0 {
		return ErrNoElements
	}

	rxPacket.RXInfoSet = rxInfoSet
	return nil
}
