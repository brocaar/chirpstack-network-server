package data

import (
	"context"

	"github.com/pkg/errors"

	"github.com/brocaar/chirpstack-network-server/internal/models"
	"github.com/brocaar/chirpstack-network-server/internal/roaming"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/backend"
)

// HandleRoamingHNS handles an uplink as a hNS.
func HandleRoamingHNS(ctx context.Context, phyPayload []byte, basePL backend.BasePayload, ulMetaData backend.ULMetaData) error {
	// decode PHYPayload
	var phy lorawan.PHYPayload
	if err := phy.UnmarshalBinary(phyPayload); err != nil {
		return errors.Wrap(err, "unmarshal phypayload error")
	}

	// convert ULMetaData to UplinkRXInfo and UplinkTXInfo
	txInfo, err := roaming.ULMetaDataToTXInfo(ulMetaData)
	if err != nil {
		return errors.Wrap(err, "up meta-data to txinfo error")
	}
	rxInfo, err := roaming.ULMetaDataToRXInfo(ulMetaData)
	if err != nil {
		return errors.Wrap(err, "ul meta-data to rxinfo error")
	}

	// Construct RXPacket
	rxPacket := models.RXPacket{
		PHYPayload: phy,
		TXInfo:     txInfo,
		RXInfoSet:  rxInfo,
		RoamingMetaData: &models.RoamingMetaData{
			BasePayload: basePL,
			ULMetaData:  ulMetaData,
		},
	}
	if ulMetaData.DataRate != nil {
		rxPacket.DR = *ulMetaData.DataRate
	}

	// Start the uplink data flow
	if err := Handle(ctx, rxPacket); err != nil {
		return errors.Wrap(err, "handle uplink error")
	}

	return nil
}
