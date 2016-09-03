package uplink

import (
	"fmt"
	"sync"

	log "github.com/Sirupsen/logrus"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/lorawan"
)

// HandleRXPackets consumes received packets by the gateway and handles them
// in a separate go-routine. Errors are logged.
func HandleRXPackets(wg sync.WaitGroup, ctx common.Context) {
	for rxPacket := range ctx.Gateway.RXPacketChan() {
		go func(rxPacket gw.RXPacket) {
			wg.Add(1)
			defer wg.Done()
			if err := HandleRXPacket(ctx, rxPacket); err != nil {
				data, _ := rxPacket.PHYPayload.MarshalText()
				log.WithField("data_base64", string(data)).Errorf("processing rx packet error: %s", err)
			}
		}(rxPacket)
	}
}

// HandleRXPacket handles a single rxpacket.
func HandleRXPacket(ctx common.Context, rxPacket gw.RXPacket) error {
	switch rxPacket.PHYPayload.MHDR.MType {
	case lorawan.JoinRequest:
		return collectJoinRequestPacket(ctx, rxPacket)
	case lorawan.UnconfirmedDataUp, lorawan.ConfirmedDataUp:
		return validateAndCollectDataUpRXPacket(ctx, rxPacket)
	default:
		return fmt.Errorf("unknown MType: %v", rxPacket.PHYPayload.MHDR.MType)
	}
}
