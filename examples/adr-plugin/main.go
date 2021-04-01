package main

import (
	"github.com/hashicorp/go-plugin"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/chirpstack-network-server/v3/adr"
)

// Type Handler is the ADR handler.
type Handler struct{}

// ID must return the plugin identifier.
func (h *Handler) ID() (string, error) {
	return "example_plugin", nil
}

// Name must return a human-readable name.
func (h *Handler) Name() (string, error) {
	return "Example ADR plugin", nil
}

// Handle handles the ADR and returns the (new) parameters.
func (h *Handler) Handle(req adr.HandleRequest) (adr.HandleResponse, error) {
	return adr.HandleResponse{
		DR:           req.DR,
		TxPowerIndex: req.TxPowerIndex,
		NbTrans:      req.NbTrans,
	}, nil
}

func main() {
	handler := &Handler{}

	pluginMap := map[string]plugin.Plugin{
		"handler": &adr.HandlerPlugin{Impl: handler},
	}

	log.Info("Starting ADR plugin")
	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: adr.HandshakeConfig,
		Plugins:         pluginMap,
	})
}
