package uplink

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/downlink"
	"github.com/brocaar/loraserver/internal/maccommand"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/backend"
)

func setContextFromJoinRequestPHYPayload(ctx *JoinRequestContext) error {
	jrPL, ok := ctx.RXPacket.PHYPayload.MACPayload.(*lorawan.JoinRequestPayload)
	if !ok {
		return fmt.Errorf("expected *lorawan.JoinRequestPayload, got: %T", ctx.RXPacket.PHYPayload.MACPayload)
	}
	ctx.JoinRequestPayload = jrPL

	return nil
}

func logJoinRequestFramesCollected(ctx *JoinRequestContext) error {
	var macs []string
	for _, p := range ctx.RXPacket.RXInfoSet {
		macs = append(macs, p.MAC.String())
	}

	log.WithFields(log.Fields{
		"dev_eui":  ctx.JoinRequestPayload.DevEUI,
		"gw_count": len(macs),
		"gw_macs":  strings.Join(macs, ", "),
		"mtype":    ctx.RXPacket.PHYPayload.MHDR.MType,
	}).Info("packet(s) collected")

	return nil
}

func getDeviceAndDeviceProfile(ctx *JoinRequestContext) error {
	var err error

	ctx.Device, err = storage.GetDevice(common.DB, ctx.JoinRequestPayload.DevEUI)
	if err != nil {
		return errors.Wrap(err, "get device error")
	}

	ctx.DeviceProfile, err = storage.GetDeviceProfile(common.DB, ctx.Device.DeviceProfileID)
	if err != nil {
		return errors.Wrap(err, "get device-profile error")
	}

	if !ctx.DeviceProfile.SupportsJoin {
		return errors.Wrap(err, "device does not support join")
	}

	return nil
}

func getRandomDevAddr(ctx *JoinRequestContext) error {
	devAddr, err := storage.GetRandomDevAddr(common.RedisPool, common.NetID)
	if err != nil {
		return errors.Wrap(err, "get random DevAddr error")
	}
	ctx.DevAddr = devAddr

	return nil
}

func getJoinAcceptFromAS(ctx *JoinRequestContext) error {
	b, err := ctx.RXPacket.PHYPayload.MarshalBinary()
	if err != nil {
		return errors.Wrap(err, "PHYPayload marshal binary error")
	}

	randomBytes := make([]byte, 4)
	_, err = rand.Read(randomBytes)
	if err != nil {
		return errors.Wrap(err, "read random bytes error")
	}
	transactionID := binary.LittleEndian.Uint32(randomBytes)

	joinReqPL := backend.JoinReqPayload{
		BasePayload: backend.BasePayload{
			ProtocolVersion: backend.ProtocolVersion1_0,
			SenderID:        common.NetID.String(),
			ReceiverID:      ctx.JoinRequestPayload.AppEUI.String(),
			TransactionID:   transactionID,
			MessageType:     backend.JoinReq,
		},
		MACVersion: ctx.DeviceProfile.MACVersion,
		PHYPayload: backend.HEXBytes(b),
		DevEUI:     ctx.JoinRequestPayload.DevEUI,
		DevAddr:    ctx.DevAddr,
		DLSettings: lorawan.DLSettings{
			RX2DataRate: uint8(ctx.DeviceProfile.RXDataRate2),
			RX1DROffset: uint8(ctx.DeviceProfile.RXDROffset1),
		},
		RxDelay: ctx.DeviceProfile.RXDelay1,
		CFList:  common.Band.GetCFList(),
	}

	jsClient, err := common.JoinServerPool.Get(ctx.JoinRequestPayload.AppEUI)
	if err != nil {
		return errors.Wrap(err, "get join-server client error")
	}

	ctx.JoinAnsPayload, err = jsClient.JoinReq(joinReqPL)
	if err != nil {
		return errors.Wrap(err, "join-request to join-server error")
	}

	return nil
}

func logJoinRequestFrame(ctx *JoinRequestContext) error {
	logUplink(common.DB, ctx.JoinRequestPayload.DevEUI, ctx.RXPacket)
	return nil
}

func createNodeSession(ctx *JoinRequestContext) error {
	if ctx.JoinAnsPayload.NwkSKey.KEKLabel != "" {
		return errors.New("NwkSKey KEKLabel unsupported")
	}

	ctx.DeviceSession = storage.DeviceSession{
		DeviceProfileID:  ctx.Device.DeviceProfileID,
		ServiceProfileID: ctx.Device.ServiceProfileID,
		RoutingProfileID: ctx.Device.RoutingProfileID,

		DevAddr:         ctx.DevAddr,
		JoinEUI:         ctx.JoinRequestPayload.AppEUI,
		DevEUI:          ctx.JoinRequestPayload.DevEUI,
		NwkSKey:         ctx.JoinAnsPayload.NwkSKey.AESKey,
		FCntUp:          0,
		FCntDown:        0,
		RXWindow:        storage.RX1,
		RXDelay:         uint8(ctx.DeviceProfile.RXDelay1),
		RX1DROffset:     uint8(ctx.DeviceProfile.RXDROffset1),
		RX2DR:           uint8(ctx.DeviceProfile.RXDataRate2),
		EnabledChannels: common.Band.GetUplinkChannels(),
		LastRXInfoSet:   ctx.RXPacket.RXInfoSet,
	}

	if err := storage.SaveDeviceSession(common.RedisPool, ctx.DeviceSession); err != nil {
		return errors.Wrap(err, "save node-session error")
	}

	if err := maccommand.FlushQueue(common.RedisPool, ctx.DeviceSession.DevEUI); err != nil {
		return fmt.Errorf("flush mac-command queue error: %s", err)
	}

	return nil
}

func sendJoinAcceptDownlink(ctx *JoinRequestContext) error {
	var phy lorawan.PHYPayload
	if err := phy.UnmarshalBinary(ctx.JoinAnsPayload.PHYPayload[:]); err != nil {
		return errors.Wrap(err, "unmarshal downlink phypayload error")
	}

	if err := downlink.Flow.RunJoinResponse(ctx.DeviceSession, phy); err != nil {
		return errors.Wrap(err, "run join-response flow error")
	}

	return nil
}
