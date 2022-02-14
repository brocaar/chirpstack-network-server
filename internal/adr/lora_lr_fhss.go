package adr

import (
	"github.com/brocaar/chirpstack-network-server/v3/adr"
	"github.com/brocaar/chirpstack-network-server/v3/internal/band"
)

// LoRaLRFHSSHandler implements a LoRa / LR-FHSS ADR handler.
type LoRaLRFHSSHandler struct{}

// ID returns the ID.
func (h *LoRaLRFHSSHandler) ID() (string, error) {
	return "lora_lr_fhss", nil
}

// Name returns the name.
func (h *LoRaLRFHSSHandler) Name() (string, error) {
	return "LoRa & LR-FHSS ADR algorithm", nil
}

// Handle handles the ADR request.
func (h *LoRaLRFHSSHandler) Handle(req adr.HandleRequest) (adr.HandleResponse, error) {
	resp := adr.HandleResponse{
		DR:           req.DR,
		TxPowerIndex: req.TxPowerIndex,
		NbTrans:      req.NbTrans,
	}

	band := band.Band()
	loRaHandler := DefaultHandler{}
	lrFHSSHandler := LRFHSSHandler{}

	loRaResp, err := loRaHandler.Handle(req)
	if err != nil {
		return resp, err
	}

	loRaDR, err := band.GetDataRate(loRaResp.DR)
	if err != nil {
		return resp, err
	}

	lrFHSSResp, err := lrFHSSHandler.Handle(req)
	if err != nil {
		return resp, err
	}

	// For SF < 10, LoRa is a better option, for SF >= 10 use LR-FHSS.
	if loRaDR.SpreadFactor < 10 {
		return loRaResp, nil
	}

	return lrFHSSResp, nil
}
