package proprietary

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/loraserver/api/as"
	"github.com/brocaar/loraserver/api/common"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/lorawan"
)

var tasks = []func(*proprietaryContext) error{
	setContextFromProprietaryPHYPayload,
	sendProprietaryPayloadToApplicationServer,
}

type proprietaryContext struct {
	RXPacket    models.RXPacket
	DataPayload *lorawan.DataPayload
}

// Handle handles a proprietary uplink frame.
func Handle(rxPacket models.RXPacket) error {
	ctx := proprietaryContext{
		RXPacket: rxPacket,
	}

	for _, t := range tasks {
		if err := t(&ctx); err != nil {
			return err
		}
	}

	return nil
}

func setContextFromProprietaryPHYPayload(ctx *proprietaryContext) error {
	dataPL, ok := ctx.RXPacket.PHYPayload.MACPayload.(*lorawan.DataPayload)
	if !ok {
		return fmt.Errorf("expected *lorawan.DataPayload, got: %T", ctx.RXPacket.PHYPayload.MACPayload)
	}
	ctx.DataPayload = dataPL
	return nil
}

func sendProprietaryPayloadToApplicationServer(ctx *proprietaryContext) error {
	var ids []lorawan.EUI64

	handleReq := as.HandleProprietaryUplinkRequest{
		MacPayload: ctx.DataPayload.Bytes,
		Mic:        ctx.RXPacket.PHYPayload.MIC[:],
		TxInfo:     ctx.RXPacket.TXInfo,
		RxInfo:     ctx.RXPacket.RXInfoSet,
	}

	// get gateway info
	for i := range handleReq.RxInfo {
		var id lorawan.EUI64
		copy(id[:], handleReq.RxInfo[i].GatewayId)
		ids = append(ids, id)
	}
	gws, err := storage.GetGatewaysForIDs(config.C.PostgreSQL.DB, ids)
	if err != nil {
		log.WithField("gateway_ids", ids).Warningf("get gateways for gateway ids error: %s", err)
		gws = make(map[lorawan.EUI64]storage.Gateway)
	}

	for i := range handleReq.RxInfo {
		var id lorawan.EUI64
		copy(id[:], handleReq.RxInfo[i].GatewayId)

		if gw, ok := gws[id]; ok {
			handleReq.RxInfo[i].Location = &common.Location{
				Latitude:  gw.Location.Latitude,
				Longitude: gw.Location.Longitude,
				Altitude:  gw.Altitude,
			}
		}
	}

	// send proprietary to all application servers, as the network-server
	// has know knowledge / state about which application-server is responsible
	// for this frame
	rps, err := storage.GetAllRoutingProfiles(config.C.PostgreSQL.DB)
	if err != nil {
		return errors.Wrap(err, "get all routing-profiles error")
	}

	for _, rp := range rps {
		go func(rp storage.RoutingProfile, handleReq as.HandleProprietaryUplinkRequest) {
			asClient, err := config.C.ApplicationServer.Pool.Get(rp.ASID, []byte(rp.CACert), []byte(rp.TLSCert), []byte(rp.TLSKey))
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
