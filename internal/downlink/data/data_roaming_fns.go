package data

import (
	"context"
	"encoding/binary"
	"github.com/brocaar/chirpstack-network-server/v3/internal/storage"
	"sort"
	"time"

	"github.com/gofrs/uuid"
	"github.com/golang/protobuf/ptypes"
	"github.com/pkg/errors"

	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/brocaar/chirpstack-network-server/v3/internal/backend/gateway"
	"github.com/brocaar/chirpstack-network-server/v3/internal/band"
	"github.com/brocaar/chirpstack-network-server/v3/internal/helpers"
	"github.com/brocaar/chirpstack-network-server/v3/internal/logging"
	"github.com/brocaar/chirpstack-network-server/v3/internal/roaming"
	"github.com/brocaar/lorawan/backend"
)

// HandleRoamingFNS handles a downlink as fNS.
func HandleRoamingFNS(ctx context.Context, pl backend.XmitDataReqPayload) error {
	// Retrieve RXInfo from DLMetaData
	rxInfo, err := roaming.DLMetaDataToUplinkRXInfoSet(*pl.DLMetaData)
	if err != nil {
		return errors.Wrap(err, "get uplink rxinfo error")
	}

	if len(rxInfo) == 0 {
		return errors.New("GWInfo must not be empty")
	}

	sort.Sort(bySignal(rxInfo))

	var downID uuid.UUID
	if ctxID := ctx.Value(logging.ContextIDKey); ctxID != nil {
		if id, ok := ctxID.(uuid.UUID); ok {
			downID = id
		}
	}

	downlink := gw.DownlinkFrame{
		GatewayId:  rxInfo[0].GatewayId,
		DownlinkId: downID[:],
		Token:      uint32(binary.BigEndian.Uint16(downID[0:2])),
		Items:      []*gw.DownlinkFrameItem{},
	}

	if pl.DLMetaData.DLFreq1 != nil && pl.DLMetaData.DataRate1 != nil && pl.DLMetaData.RXDelay1 != nil {
		item := gw.DownlinkFrameItem{
			PhyPayload: pl.PHYPayload[:],
			TxInfo: &gw.DownlinkTXInfo{
				Frequency: uint32(*pl.DLMetaData.DLFreq1 * 1000000),
				Board:     rxInfo[0].Board,
				Antenna:   rxInfo[0].Antenna,
				Context:   rxInfo[0].Context,
				Timing:    gw.DownlinkTiming_DELAY,
				TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
					DelayTimingInfo: &gw.DelayTimingInfo{
						Delay: ptypes.DurationProto(time.Duration(*pl.DLMetaData.RXDelay1) * time.Second),
					},
				},
			},
		}

		item.TxInfo.Power = int32(band.Band().GetDownlinkTXPower(item.TxInfo.Frequency))

		if err := helpers.SetDownlinkTXInfoDataRate(item.TxInfo, *pl.DLMetaData.DataRate1, band.Band()); err != nil {
			return errors.Wrap(err, "set downlink txinfo data-rate error")
		}

		downlink.Items = append(downlink.Items, &item)
	}

	if pl.DLMetaData.DLFreq2 != nil && pl.DLMetaData.DataRate2 != nil && pl.DLMetaData.RXDelay1 != nil {
		item := gw.DownlinkFrameItem{
			PhyPayload: pl.PHYPayload[:],
			TxInfo: &gw.DownlinkTXInfo{
				Frequency: uint32(*pl.DLMetaData.DLFreq2 * 1000000),
				Board:     rxInfo[0].Board,
				Antenna:   rxInfo[0].Antenna,
				Context:   rxInfo[0].Context,
				Timing:    gw.DownlinkTiming_DELAY,
				TimingInfo: &gw.DownlinkTXInfo_DelayTimingInfo{
					DelayTimingInfo: &gw.DelayTimingInfo{
						Delay: ptypes.DurationProto(time.Duration(*pl.DLMetaData.RXDelay1+1) * time.Second),
					},
				},
			},
		}

		item.TxInfo.Power = int32(band.Band().GetDownlinkTXPower(item.TxInfo.Frequency))

		if err := helpers.SetDownlinkTXInfoDataRate(item.TxInfo, *pl.DLMetaData.DataRate2, band.Band()); err != nil {
			return errors.Wrap(err, "set downlink txinfo data-rate error")
		}

		downlink.Items = append(downlink.Items, &item)
	}

	df := storage.DownlinkFrame{
		Token:         downlink.Token,
		DownlinkFrame: &downlink,
	}

	if err := storage.SaveDownlinkFrame(ctx, &df); err != nil {
		return errors.Wrap(err, "save downlink-frame error")
	}

	if err := gateway.Backend().SendTXPacket(downlink); err != nil {
		return errors.Wrap(err, "send downlink-frame to gateway error")
	}

	return nil
}

type bySignal []*gw.UplinkRXInfo

func (s bySignal) Len() int {
	return len(s)
}

func (s bySignal) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s bySignal) Less(i, j int) bool {
	if s[i].LoraSnr == s[j].LoraSnr {
		return s[i].Rssi > s[j].Rssi
	}

	return s[i].LoraSnr > s[j].LoraSnr
}
