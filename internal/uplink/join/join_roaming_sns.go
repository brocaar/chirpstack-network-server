package join

import (
	"context"

	"github.com/pkg/errors"

	"github.com/brocaar/chirpstack-network-server/v3/internal/models"
	"github.com/brocaar/lorawan/backend"
)

// HandleStartPR handles starting a passive-roaming OTAA activation as the
// hNS.
func HandleStartPRHNS(ctx context.Context, prStartPL backend.PRStartReqPayload, rxPacket models.RXPacket) (backend.PRStartAnsPayload, error) {
	jctx := joinContext{
		ctx:               ctx,
		RXPacket:          rxPacket,
		PRStartReqPayload: &prStartPL,
	}

	for _, f := range []func() error{
		jctx.setContextFromJoinRequestPHYPayload,
		jctx.logJoinRequestFramesCollected,
		jctx.getDeviceOrTryRoaming,
		jctx.getDeviceProfile,
		jctx.getServiceProfile,
		jctx.abortOnDeviceIsDisabled,
		jctx.validateNonce,
		jctx.getRandomDevAddr,
		jctx.getJoinAcceptFromAS,
		jctx.sendUplinkMetaDataToNetworkController,
		jctx.flushDeviceQueue,
		jctx.createDeviceSession,
		jctx.createDeviceActivation,
		jctx.setDeviceMode,
		jctx.setPRStartAnsPayload,
	} {
		if err := f(); err != nil {
			return backend.PRStartAnsPayload{}, err
		}
	}

	if jctx.PRStartAnsPayload != nil {
		return *jctx.PRStartAnsPayload, nil
	}

	return backend.PRStartAnsPayload{}, errors.New("PRStartAnsPayload is not set")
}
