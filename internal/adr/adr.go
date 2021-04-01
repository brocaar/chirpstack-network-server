package adr

import (
	"fmt"
	"os/exec"

	"github.com/hashicorp/go-plugin"
	"github.com/pkg/errors"

	"github.com/brocaar/chirpstack-network-server/v3/adr"
	"github.com/brocaar/chirpstack-network-server/v3/internal/config"
)

var (
	handlers     map[string]adr.Handler
	handlerNames map[string]string
)

func init() {
	def := &DefaultHandler{}
	id, _ := def.ID()
	name, _ := def.Name()

	handlers = map[string]adr.Handler{
		id: def,
	}

	handlerNames = map[string]string{
		id: name,
	}
}

// Setup configures the ADR package.
func Setup(conf config.Config) error {
	for _, adrPlugin := range conf.NetworkServer.NetworkSettings.ADRPlugins {
		client := plugin.NewClient(&plugin.ClientConfig{
			HandshakeConfig: adr.HandshakeConfig,
			Plugins: map[string]plugin.Plugin{
				"handler": &adr.HandlerPlugin{},
			},
			Cmd: exec.Command(adrPlugin),
		})

		// connect via RPC
		rpcClient, err := client.Client()
		if err != nil {
			return errors.Wrap(err, "plugin rpc client error")
		}

		// request the plugin
		raw, err := rpcClient.Dispense("handler")
		if err != nil {
			return errors.Wrap(err, "request handler plugin error")
		}

		// cast to Handler.
		handler, ok := raw.(adr.Handler)
		if !ok {
			return fmt.Errorf("expected adr.Handler, got: %T", raw)
		}

		// get ID.
		id, err := handler.ID()
		if err != nil {
			return errors.Wrap(err, "get plugin id error")
		}

		// get Name.
		name, err := handler.Name()
		if err != nil {
			return errors.Wrap(err, "get plugin name error")
		}

		handlers[id] = handler
		handlerNames[id] = name
	}

	return nil
}

// GetHandler returns the ADR handler by its ID, failing that it returns the
// default ADR handler.
func GetHandler(id string) adr.Handler {
	h, ok := handlers[id]
	if !ok {
		return &DefaultHandler{}
	}

	return h
}

// GetADRAlgorithms returns the available ADR algorithms.
func GetADRAlgorithms() map[string]string {
	return handlerNames
}
