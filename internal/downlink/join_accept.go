package downlink

import (
	"fmt"
	"time"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/session"
	"github.com/brocaar/lorawan"
)

func getJoinAcceptTXInfo(ns session.NodeSession, rxInfo gw.RXInfo) (gw.TXInfo, error) {
	txInfo := gw.TXInfo{
		MAC:      rxInfo.MAC,
		CodeRate: rxInfo.CodeRate,
		Power:    common.Band.DefaultTXPower,
	}

	if ns.RXWindow == session.RX1 {
		txInfo.Timestamp = rxInfo.Timestamp + uint32(common.Band.JoinAcceptDelay1/time.Microsecond)

		// get uplink dr
		uplinkDR, err := common.Band.GetDataRate(rxInfo.DataRate)
		if err != nil {
			return txInfo, err
		}

		// get RX1 DR
		rx1DR, err := common.Band.GetRX1DataRate(uplinkDR, 0)
		if err != nil {
			return txInfo, err

		}
		txInfo.DataRate = common.Band.DataRates[rx1DR]

		// get RX1 frequency
		txInfo.Frequency, err = common.Band.GetRX1Frequency(rxInfo.Frequency)
		if err != nil {
			return txInfo, err
		}
	} else if ns.RXWindow == session.RX2 {
		txInfo.Timestamp = rxInfo.Timestamp + uint32(common.Band.JoinAcceptDelay2/time.Microsecond)
		txInfo.DataRate = common.Band.DataRates[common.Band.RX2DataRate]
		txInfo.Frequency = common.Band.RX2Frequency
	} else {
		return txInfo, fmt.Errorf("unkonwn RXWindow option %d", ns.RXWindow)
	}
	return txInfo, nil
}

// SendJoinAcceptResponse sends the join-accept response.
func SendJoinAcceptResponse(ns session.NodeSession, rxPacket models.RXPacket, phy lorawan.PHYPayload) error {
	txInfo, err := getJoinAcceptTXInfo(ns, rxPacket.RXInfoSet[0])
	if err != nil {
		return fmt.Errorf("get join-accept txinfo error: %s", err)
	}

	logDownlink(common.DB, ns.DevEUI, phy, txInfo)

	if err = common.Gateway.SendTXPacket(gw.TXPacket{
		TXInfo:     txInfo,
		PHYPayload: phy,
	}); err != nil {
		return fmt.Errorf("send txpacket error: %s", err)
	}
	return nil
}
