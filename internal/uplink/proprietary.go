package uplink

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/loraserver/api/as"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/gateway"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/lorawan"
)

func setContextFromProprietaryPHYPayload(ctx *ProprietaryUpContext) error {
	dataPL, ok := ctx.RXPacket.PHYPayload.MACPayload.(*lorawan.DataPayload)
	if !ok {
		return fmt.Errorf("expected *lorawan.DataPayload, got: %T", ctx.RXPacket.PHYPayload.MACPayload)
	}
	ctx.DataPayload = dataPL
	return nil
}

func sendProprietaryPayloadToApplicationServer(ctx *ProprietaryUpContext) error {
	handleReq := as.HandleProprietaryUplinkRequest{
		MacPayload: ctx.DataPayload.Bytes,
		Mic:        ctx.RXPacket.PHYPayload.MIC[:],
		TxInfo: &as.TXInfo{
			Frequency: int64(ctx.RXPacket.RXInfoSet[0].Frequency),
			CodeRate:  ctx.RXPacket.RXInfoSet[0].CodeRate,
			DataRate: &as.DataRate{
				Modulation:   string(ctx.RXPacket.RXInfoSet[0].DataRate.Modulation),
				BandWidth:    uint32(ctx.RXPacket.RXInfoSet[0].DataRate.Bandwidth),
				SpreadFactor: uint32(ctx.RXPacket.RXInfoSet[0].DataRate.SpreadFactor),
				Bitrate:      uint32(ctx.RXPacket.RXInfoSet[0].DataRate.BitRate),
			},
		},
	}

	var macs []lorawan.EUI64
	for i := range ctx.RXPacket.RXInfoSet {
		macs = append(macs, ctx.RXPacket.RXInfoSet[i].MAC)
	}

	// get gateway info
	gws, err := gateway.GetGatewaysForMACs(common.DB, macs)
	if err != nil {
		log.WithField("macs", macs).Warningf("get gateways for macs error: %s", err)
		gws = make(map[lorawan.EUI64]gateway.Gateway)
	}

	for _, rxInfo := range ctx.RXPacket.RXInfoSet {
		// make sure we have a copy of the MAC byte slice, else every RxInfo
		// slice item will get the same Mac
		mac := make([]byte, 8)
		copy(mac, rxInfo.MAC[:])

		asRxInfo := as.RXInfo{
			Mac:     mac,
			Rssi:    int32(rxInfo.RSSI),
			LoRaSNR: rxInfo.LoRaSNR,
		}

		if rxInfo.Time != nil {
			asRxInfo.Time = rxInfo.Time.Format(time.RFC3339Nano)
		}

		if gw, ok := gws[rxInfo.MAC]; ok {
			asRxInfo.Name = gw.Name
			asRxInfo.Latitude = gw.Location.Latitude
			asRxInfo.Longitude = gw.Location.Longitude
			asRxInfo.Altitude = gw.Altitude
		}

		handleReq.RxInfo = append(handleReq.RxInfo, &asRxInfo)
	}

	// send proprietary to all application servers, as the network-server
	// has know knowledge / state about which application-server is responsible
	// for this frame
	rps, err := storage.GetAllRoutingProfiles(common.DB)
	if err != nil {
		return errors.Wrap(err, "get all routing-profiles error")
	}

	for _, rp := range rps {
		go func(rp storage.RoutingProfile, handleReq as.HandleProprietaryUplinkRequest) {
			asClient, err := common.ApplicationServerPool.Get(rp.ASID)
			if err != nil {
				log.WithError(err).Error("get application-server client error")
				return
			}

			if _, err = asClient.HandleProprietaryUplink(context.Background(), &handleReq); err != nil {
				log.WithError(err).Error("handle proprietary up error")
				return
			}
		}(rp, handleReq)
	}

	return nil
}
