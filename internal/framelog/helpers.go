package framelog

import (
	"time"

	"github.com/brocaar/loraserver/internal/models"

	"github.com/brocaar/loraserver/api/common"
	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/band"
	"github.com/golang/protobuf/ptypes"
	"github.com/pkg/errors"
)

// CreateDownlinkFrame is a helper until gw.DownlinkFrame is fully in place.
func CreateDownlinkFrame(token uint16, phy lorawan.PHYPayload, txInfo gw.TXInfo) (gw.DownlinkFrame, error) {
	b, err := phy.MarshalBinary()
	if err != nil {
		return gw.DownlinkFrame{}, errors.Wrap(err, "marshal phypayload error")
	}

	downlinkFrame := gw.DownlinkFrame{
		PhyPayload: b,
		Token:      uint32(token),
		TxInfo: &gw.DownlinkTXInfo{
			GatewayId:   txInfo.MAC[:],
			Immediately: txInfo.Immediately,
			Frequency:   uint32(txInfo.Frequency),
			Power:       int32(txInfo.Power),
			Board:       uint32(txInfo.Board),
			Antenna:     uint32(txInfo.Antenna),
		},
	}

	if txInfo.TimeSinceGPSEpoch != nil {
		downlinkFrame.TxInfo.TimeSinceGpsEpoch = ptypes.DurationProto(time.Duration(*txInfo.TimeSinceGPSEpoch))
	}

	if txInfo.Timestamp != nil {
		downlinkFrame.TxInfo.Timestamp = *txInfo.Timestamp
	}

	switch txInfo.DataRate.Modulation {
	case band.LoRaModulation:
		downlinkFrame.TxInfo.Modulation = common.Modulation_LORA
		downlinkFrame.TxInfo.ModulationInfo = &gw.DownlinkTXInfo_LoraModulationInfo{
			LoraModulationInfo: &gw.LoRaModulationInfo{
				Bandwidth:             uint32(txInfo.DataRate.Bandwidth),
				SpreadingFactor:       uint32(txInfo.DataRate.SpreadFactor),
				CodeRate:              txInfo.CodeRate,
				PolarizationInversion: true,
			},
		}
	case band.FSKModulation:
		downlinkFrame.TxInfo.Modulation = common.Modulation_FSK
		downlinkFrame.TxInfo.ModulationInfo = &gw.DownlinkTXInfo_FskModulationInfo{
			FskModulationInfo: &gw.FSKModulationInfo{
				Bandwidth: uint32(txInfo.DataRate.Bandwidth),
				Bitrate:   uint32(txInfo.DataRate.BitRate),
			},
		}
	}

	return downlinkFrame, nil
}

// CreateUplinkFrameSet is a helper until gw.UplinkFrameSet is fully in place.
func CreateUplinkFrameSet(rxPacket models.RXPacket) (gw.UplinkFrameSet, error) {
	b, err := rxPacket.PHYPayload.MarshalBinary()
	if err != nil {
		return gw.UplinkFrameSet{}, errors.Wrap(err, "marshal phypayload error")
	}

	return gw.UplinkFrameSet{
		PhyPayload: b,
		TxInfo:     rxPacket.GetGWUplinkTXInfo(),
		RxInfo:     rxPacket.GetGWUplinkRXInfoSet(),
	}, nil
}
