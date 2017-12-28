package join

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/loraserver/internal/common"
	joindown "github.com/brocaar/loraserver/internal/downlink/join"
	"github.com/brocaar/loraserver/internal/maccommand"
	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/node"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/backend"
)

var tasks = []func(*context) error{
	setContextFromJoinRequestPHYPayload,
	logJoinRequestFramesCollected,
	getDeviceAndDeviceProfile,
	validateNonce,
	getRandomDevAddr,
	getJoinAcceptFromAS,
	logJoinRequestFrame,
	flushDeviceQueue,
	createNodeSession,
	createDeviceActivation,
	sendJoinAcceptDownlink,
}

type context struct {
	RXPacket           models.RXPacket
	JoinRequestPayload *lorawan.JoinRequestPayload
	Device             storage.Device
	ServiceProfile     storage.ServiceProfile
	DeviceProfile      storage.DeviceProfile
	DevAddr            lorawan.DevAddr
	CFList             []uint32
	JoinAnsPayload     backend.JoinAnsPayload
	DeviceSession      storage.DeviceSession
}

// Handle handles a join-request
func Handle(rxPacket models.RXPacket) error {
	ctx := context{
		RXPacket: rxPacket,
	}

	for _, t := range tasks {
		if err := t(&ctx); err != nil {
			return err
		}
	}

	return nil
}

func setContextFromJoinRequestPHYPayload(ctx *context) error {
	jrPL, ok := ctx.RXPacket.PHYPayload.MACPayload.(*lorawan.JoinRequestPayload)
	if !ok {
		return fmt.Errorf("expected *lorawan.JoinRequestPayload, got: %T", ctx.RXPacket.PHYPayload.MACPayload)
	}
	ctx.JoinRequestPayload = jrPL

	return nil
}

func logJoinRequestFramesCollected(ctx *context) error {
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

func getDeviceAndDeviceProfile(ctx *context) error {
	var err error

	ctx.Device, err = storage.GetDevice(common.DB, ctx.JoinRequestPayload.DevEUI)
	if err != nil {
		return errors.Wrap(err, "get device error")
	}

	ctx.DeviceProfile, err = storage.GetDeviceProfile(common.DB, ctx.Device.DeviceProfileID)
	if err != nil {
		return errors.Wrap(err, "get device-profile error")
	}

	ctx.ServiceProfile, err = storage.GetServiceProfile(common.DB, ctx.Device.ServiceProfileID)
	if err != nil {
		return errors.Wrap(err, "get service-profile error")
	}

	if !ctx.DeviceProfile.SupportsJoin {
		return errors.Wrap(err, "device does not support join")
	}

	return nil
}

func validateNonce(ctx *context) error {
	// validate that the nonce has not been used yet
	err := storage.ValidateDevNonce(common.DB, ctx.JoinRequestPayload.AppEUI, ctx.JoinRequestPayload.DevEUI, ctx.JoinRequestPayload.DevNonce)
	if err != nil {
		return errors.Wrap(err, "validate dev-nonce error")
	}

	return nil
}

func getRandomDevAddr(ctx *context) error {
	devAddr, err := storage.GetRandomDevAddr(common.RedisPool, common.NetID)
	if err != nil {
		return errors.Wrap(err, "get random DevAddr error")
	}
	ctx.DevAddr = devAddr

	return nil
}

func getJoinAcceptFromAS(ctx *context) error {
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
			RX2DataRate: uint8(common.RX2DR),
			RX1DROffset: uint8(common.RX1DROffset),
		},
		RxDelay: common.RX1Delay,
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

func logJoinRequestFrame(ctx *context) error {
	logUplink(common.DB, ctx.JoinRequestPayload.DevEUI, ctx.RXPacket)
	return nil
}

func flushDeviceQueue(ctx *context) error {
	if err := storage.FlushDeviceQueueForDevEUI(common.DB, ctx.Device.DevEUI); err != nil {
		return errors.Wrap(err, "flush device-queue error")
	}
	return nil
}

func createNodeSession(ctx *context) error {
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
		RXDelay:         uint8(common.RX1Delay),
		RX1DROffset:     uint8(common.RX1DROffset),
		RX2DR:           uint8(common.RX2DR),
		EnabledChannels: common.Band.GetUplinkChannels(),
		LastRXInfoSet:   ctx.RXPacket.RXInfoSet,
		MaxSupportedDR:  ctx.ServiceProfile.ServiceProfile.DRMax,
	}

	if err := storage.SaveDeviceSession(common.RedisPool, ctx.DeviceSession); err != nil {
		return errors.Wrap(err, "save node-session error")
	}

	if err := maccommand.FlushQueue(common.RedisPool, ctx.DeviceSession.DevEUI); err != nil {
		return fmt.Errorf("flush mac-command queue error: %s", err)
	}

	return nil
}

func createDeviceActivation(ctx *context) error {
	da := storage.DeviceActivation{
		DevEUI:   ctx.DeviceSession.DevEUI,
		JoinEUI:  ctx.DeviceSession.JoinEUI,
		DevAddr:  ctx.DeviceSession.DevAddr,
		NwkSKey:  ctx.DeviceSession.NwkSKey,
		DevNonce: ctx.JoinRequestPayload.DevNonce,
	}

	if err := storage.CreateDeviceActivation(common.DB, &da); err != nil {
		return errors.Wrap(err, "create device-activation error")
	}

	return nil
}

func sendJoinAcceptDownlink(ctx *context) error {
	var phy lorawan.PHYPayload
	if err := phy.UnmarshalBinary(ctx.JoinAnsPayload.PHYPayload[:]); err != nil {
		return errors.Wrap(err, "unmarshal downlink phypayload error")
	}

	if err := joindown.Handle(ctx.DeviceSession, phy); err != nil {
		return errors.Wrap(err, "run join-response flow error")
	}

	return nil
}

func logUplink(db *sqlx.DB, devEUI lorawan.EUI64, rxPacket models.RXPacket) {
	if !common.LogNodeFrames {
		return
	}

	phyB, err := rxPacket.PHYPayload.MarshalBinary()
	if err != nil {
		log.Errorf("marshal phypayload to binary error: %s", err)
		return
	}

	rxB, err := json.Marshal(rxPacket.RXInfoSet)
	if err != nil {
		log.Errorf("marshal rx-info set to json error: %s", err)
		return
	}

	fl := node.FrameLog{
		DevEUI:     devEUI,
		RXInfoSet:  &rxB,
		PHYPayload: phyB,
	}
	err = node.CreateFrameLog(db, &fl)
	if err != nil {
		log.Errorf("create frame-log error: %s", err)
	}
}
