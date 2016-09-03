package uplink

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"

	"github.com/brocaar/loraserver/api/as"
	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/api/nc"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/downlink"
	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/session"
	"github.com/brocaar/lorawan"
)

func validateAndCollectDataUpRXPacket(ctx common.Context, rxPacket gw.RXPacket) error {
	// MACPayload must be of type *lorawan.MACPayload
	macPL, ok := rxPacket.PHYPayload.MACPayload.(*lorawan.MACPayload)
	if !ok {
		return fmt.Errorf("expected *lorawan.MACPayload, got: %T", rxPacket.PHYPayload.MACPayload)
	}

	// get the session data
	ns, err := session.GetNodeSession(ctx.RedisPool, macPL.FHDR.DevAddr)
	if err != nil {
		return err
	}

	// validate and get the full int32 FCnt
	fullFCnt, ok := session.ValidateAndGetFullFCntUp(ns, macPL.FHDR.FCnt)
	if !ok {
		ctx.Application.PublishError(context.Background(), &as.PublishErrorRequest{
			DevEUI: ns.DevEUI[:],
			Type:   as.ErrorType_DATA_UP_FCNT,
			Error:  fmt.Sprintf("invalid FCnt or too many dropped frames (server_fcnt: %d, packet_fcnt: %d)", ns.FCntUp, macPL.FHDR.FCnt),
		})
		log.WithFields(log.Fields{
			"dev_addr":    macPL.FHDR.DevAddr,
			"dev_eui":     ns.DevEUI,
			"packet_fcnt": macPL.FHDR.FCnt,
			"server_fcnt": ns.FCntUp,
		}).Warning("invalid FCnt")
		return errors.New("invalid FCnt or too many dropped frames")
	}
	macPL.FHDR.FCnt = fullFCnt

	// validate MIC
	micOK, err := rxPacket.PHYPayload.ValidateMIC(ns.NwkSKey)
	if err != nil {
		return fmt.Errorf("validate MIC error: %s", err)
	}
	if !micOK {
		ctx.Application.PublishError(context.Background(), &as.PublishErrorRequest{
			DevEUI: ns.DevEUI[:],
			Type:   as.ErrorType_DATA_UP_MIC,
			Error:  "invalid MIC",
		})
		return errors.New("invalid MIC")
	}

	if macPL.FPort != nil {
		if *macPL.FPort == 0 {
			// decrypt FRMPayload with NwkSKey when FPort == 0
			if err := rxPacket.PHYPayload.DecryptFRMPayload(ns.NwkSKey); err != nil {
				return fmt.Errorf("decrypt FRMPayload error: %s", err)
			}
		}
	}

	return collectAndCallOnce(ctx.RedisPool, rxPacket, func(rxPackets models.RXPackets) error {
		return handleCollectedDataUpPackets(ctx, rxPackets)
	})
}

func handleCollectedDataUpPackets(ctx common.Context, rxPackets models.RXPackets) error {
	if len(rxPackets) == 0 {
		return errors.New("packet collector returned 0 packets")
	}
	rxPacket := rxPackets[0]

	var macs []string
	for _, p := range rxPackets {
		macs = append(macs, p.RXInfo.MAC.String())
	}

	macPL, ok := rxPacket.PHYPayload.MACPayload.(*lorawan.MACPayload)
	if !ok {
		return fmt.Errorf("expected *lorawan.MACPayload, got: %T", rxPacket.PHYPayload.MACPayload)
	}

	ns, err := session.GetNodeSession(ctx.RedisPool, macPL.FHDR.DevAddr)
	if err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"dev_eui":  ns.DevEUI,
		"gw_count": len(rxPackets),
		"gw_macs":  strings.Join(macs, ", "),
		"mtype":    rxPackets[0].PHYPayload.MHDR.MType,
	}).Info("packet(s) collected")

	// send rx info notification to be used by the network-controller
	if err = sendRXInfoPayload(ctx, ns.DevEUI, rxPackets); err != nil {
		log.WithFields(log.Fields{
			"dev_eui": ns.DevEUI,
		}).Errorf("send rx info to network-controller error: %s", err)
	}

	// handle FOpts mac commands (if any)
	if len(macPL.FHDR.FOpts) > 0 {
		if err := handleUplinkMACCommands(ctx, ns.DevEUI, false, macPL.FHDR.FOpts); err != nil {
			log.WithFields(log.Fields{
				"dev_eui": ns.DevEUI,
				"fopts":   macPL.FHDR.FOpts,
			}).Errorf("handle FOpts mac commands error: %s", err)
		}
	}

	if macPL.FPort != nil {
		if *macPL.FPort == 0 {
			if len(macPL.FRMPayload) == 0 {
				return errors.New("expected mac commands, but FRMPayload is empty (FPort=0)")
			}

			// since the PHYPayload has been marshaled / unmarshaled when
			// storing it into and retrieving it from the database, we need
			// to decode the MAC commands from the FRMPayload.
			if err = rxPacket.PHYPayload.DecodeFRMPayloadToMACCommands(); err != nil {
				return fmt.Errorf("decode FRMPayload field to MACCommand items error: %s", err)
			}

			var commands []lorawan.MACCommand
			for _, pl := range macPL.FRMPayload {
				cmd, ok := pl.(*lorawan.MACCommand)
				if !ok {
					return fmt.Errorf("expected MACPayload, but got %T", macPL.FRMPayload)
				}
				commands = append(commands, *cmd)
			}
			if err := handleUplinkMACCommands(ctx, ns.DevEUI, true, commands); err != nil {
				log.WithFields(log.Fields{
					"dev_eui":  ns.DevEUI,
					"commands": commands,
				}).Errorf("handle FRMPayload mac commands error: %s", err)
			}
		} else {
			if err := publishDataUp(ctx, ns.DevEUI, rxPackets, *macPL); err != nil {
				return err
			}
		}
	}

	// sync counter with that of the device
	ns.FCntUp = macPL.FHDR.FCnt
	if err := session.SaveNodeSession(ctx.RedisPool, ns); err != nil {
		return err
	}

	// handle uplink ACK
	if macPL.FHDR.FCtrl.ACK {
		if err := handleUplinkACK(ctx, &ns); err != nil {
			return fmt.Errorf("handle uplink ack error: %s", err)
		}
	}

	// handle downlink (ACK)
	time.Sleep(CollectDataDownWait)
	if err := downlink.SendDataDownResponse(ctx, ns, rxPackets); err != nil {
		return fmt.Errorf("handling downlink data for node %s failed: %s", ns.DevEUI, err)
	}

	return nil
}

// sendRXInfoPayload sends the rx and tx meta-data to the network controller.
func sendRXInfoPayload(ctx common.Context, devEUI lorawan.EUI64, rxPackets models.RXPackets) error {
	if len(rxPackets) == 0 {
		return fmt.Errorf("length of rx packets must be at least 1")
	}

	macPL, ok := rxPackets[0].PHYPayload.MACPayload.(*lorawan.MACPayload)
	if !ok {
		return fmt.Errorf("expected *lorawan.MACPayload, got: %T", rxPackets[0].PHYPayload.MACPayload)
	}

	rxInfoReq := nc.PublishRXInfoRequest{
		DevEUI: devEUI[:],
		TxInfo: &nc.TXInfo{
			Frequency: int64(rxPackets[0].RXInfo.Frequency),
			Adr:       macPL.FHDR.FCtrl.ADR,
			CodeRate:  rxPackets[0].RXInfo.CodeRate,
			DataRate: &nc.DataRate{
				Modulation:   string(rxPackets[0].RXInfo.DataRate.Modulation),
				BandWidth:    uint32(rxPackets[0].RXInfo.DataRate.Bandwidth),
				SpreadFactor: uint32(rxPackets[0].RXInfo.DataRate.SpreadFactor),
				Bitrate:      uint32(rxPackets[0].RXInfo.DataRate.BitRate),
			},
		},
	}

	for _, rxPacket := range rxPackets {
		rxInfoReq.RxInfo = append(rxInfoReq.RxInfo, &nc.RXInfo{
			Mac:     rxPacket.RXInfo.MAC[:],
			Time:    rxPacket.RXInfo.Time.String(),
			Rssi:    int32(rxPacket.RXInfo.RSSI),
			LoRaSNR: rxPacket.RXInfo.LoRaSNR,
		})
	}

	_, err := ctx.Controller.PublishRXInfo(context.Background(), &rxInfoReq)
	if err != nil {
		return fmt.Errorf("publish rxinfo to network-controller error: %s", err)
	}
	log.WithFields(log.Fields{
		"dev_eui": devEUI,
	}).Info("rx info sent to network-controller")
	return nil
}

func publishDataUp(ctx common.Context, devEUI lorawan.EUI64, rxPackets models.RXPackets, macPL lorawan.MACPayload) error {
	if len(rxPackets) == 0 {
		return fmt.Errorf("length of rx packets must be at least 1")
	}

	publishDataUpReq := as.PublishDataUpRequest{
		DevEUI: devEUI[:],
		FCnt:   macPL.FHDR.FCnt,
		TxInfo: &as.TXInfo{
			Frequency: int64(rxPackets[0].RXInfo.Frequency),
			Adr:       macPL.FHDR.FCtrl.ADR,
			CodeRate:  rxPackets[0].RXInfo.CodeRate,
			DataRate: &as.DataRate{
				Modulation:   string(rxPackets[0].RXInfo.DataRate.Modulation),
				BandWidth:    uint32(rxPackets[0].RXInfo.DataRate.Bandwidth),
				SpreadFactor: uint32(rxPackets[0].RXInfo.DataRate.SpreadFactor),
				Bitrate:      uint32(rxPackets[0].RXInfo.DataRate.BitRate),
			},
		},
	}

	for _, rxPacket := range rxPackets {
		publishDataUpReq.RxInfo = append(publishDataUpReq.RxInfo, &as.RXInfo{
			Mac:     rxPacket.RXInfo.MAC[:],
			Time:    rxPacket.RXInfo.Time.String(),
			Rssi:    int32(rxPacket.RXInfo.RSSI),
			LoRaSNR: rxPacket.RXInfo.LoRaSNR,
		})
	}

	if macPL.FPort != nil {
		publishDataUpReq.FPort = uint32(*macPL.FPort)
	}

	if len(macPL.FRMPayload) == 1 {
		dataPL, ok := macPL.FRMPayload[0].(*lorawan.DataPayload)
		if !ok {
			return fmt.Errorf("expected type *lorawan.DataPayload, got %T", macPL.FRMPayload[0])
		}
		publishDataUpReq.Data = dataPL.Bytes

	}

	if _, err := ctx.Application.PublishDataUp(context.Background(), &publishDataUpReq); err != nil {
		return fmt.Errorf("publish data up to application-server error: %s", err)
	}
	return nil
}

func handleUplinkMACCommands(ctx common.Context, devEUI lorawan.EUI64, frmPayload bool, commands []lorawan.MACCommand) error {
	for _, cmd := range commands {
		logFields := log.Fields{
			"dev_eui":     devEUI,
			"cid":         cmd.CID,
			"frm_payload": frmPayload,
		}

		b, err := cmd.MarshalBinary()
		if err != nil {
			return fmt.Errorf("binary marshal mac command error: %s", err)
		}

		_, err = ctx.Controller.PublishDataUpMACCommand(context.Background(), &nc.PublishDataUpMACCommandRequest{
			DevEUI:     devEUI[:],
			FrmPayload: frmPayload,
			Data:       b,
		})
		if err != nil {
			return fmt.Errorf("send mac-commnad to network-controller error: %s", err)
		}
		log.WithFields(logFields).Info("mac-command sent to network-controller")
	}
	return nil
}

func handleUplinkACK(ctx common.Context, ns *session.NodeSession) error {
	_, err := ctx.Application.PublishDataDownACK(context.Background(), &as.PublishDataDownACKRequest{
		DevEUI: ns.DevEUI[:],
		FCnt:   ns.FCntDown,
	})
	if err != nil {
		return fmt.Errorf("error publish downlink data ack to application-server: %s", err)
	}
	ns.FCntDown++
	if err = session.SaveNodeSession(ctx.RedisPool, *ns); err != nil {
		return err
	}
	return nil
}
