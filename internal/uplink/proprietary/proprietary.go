package proprietary

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/chirpstack-api/go/v3/as"
	"github.com/brocaar/chirpstack-api/go/v3/common"
	"github.com/brocaar/chirpstack-network-server/internal/backend/applicationserver"
	"github.com/brocaar/chirpstack-network-server/internal/logging"
	"github.com/brocaar/chirpstack-network-server/internal/models"
	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/brocaar/lorawan"
)

var tasks = []func(*proprietaryContext) error{
	setContextFromProprietaryPHYPayload,
	sendProprietaryPayloadToApplicationServer,
}

type proprietaryContext struct {
	ctx context.Context

	RXPacket    models.RXPacket
	DataPayload *lorawan.DataPayload
}

// Handle handles a proprietary uplink frame.
func Handle(ctx context.Context, rxPacket models.RXPacket) error {
	pctx := proprietaryContext{
		ctx:      ctx,
		RXPacket: rxPacket,
	}

	for _, t := range tasks {
		if err := t(&pctx); err != nil {
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
	gws, err := storage.GetGatewaysForIDs(ctx.ctx, storage.DB(), ids)
	if err != nil {
		log.WithFields(log.Fields{
			"gateway_ids": ids,
			"ctx_id":      ctx.ctx.Value(logging.ContextIDKey),
		}).Warningf("get gateways for gateway ids error: %s", err)
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
	rps, err := storage.GetAllRoutingProfiles(ctx.ctx, storage.DB())
	if err != nil {
		return errors.Wrap(err, "get all routing-profiles error")
	}

	for _, rp := range rps {
		go func(ctx context.Context, rp storage.RoutingProfile, handleReq as.HandleProprietaryUplinkRequest) {
			asClient, err := applicationserver.Pool().Get(rp.ASID, []byte(rp.CACert), []byte(rp.TLSCert), []byte(rp.TLSKey))
			if err != nil {
				log.WithError(err).Error("get application-server client error")
				return
			}

			if _, err = asClient.HandleProprietaryUplink(ctx, &handleReq); err != nil {
				log.WithFields(log.Fields{
					"ctx_id": ctx.Value(logging.ContextIDKey),
				}).WithError(err).Error("handle proprietary up error")
				return
			}
		}(ctx.ctx, rp, handleReq)
	}

	return nil
}
