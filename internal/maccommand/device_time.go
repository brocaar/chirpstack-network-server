package maccommand

import (
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/loraserver/internal/gps"
	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/lorawan"
)

// handleDeviceTimeReq returns the timestamp after the RX was completed.
// It will try in the following order:
//   * TimeSinceGpsEpoch field
//   * Time field
//   * Current server time
//
// Note that the last case is a fallback to at least return something. With a
// high latency between the gateway and the network-server, this timestamp
// might not be accurate.
func handleDeviceTimeReq(ds *storage.DeviceSession, rxPacket models.RXPacket) ([]storage.MACCommandBlock, error) {
	if len(rxPacket.RXInfoSet) == 0 {
		return nil, errors.New("rx info-set contains zero items")
	}

	var err error
	var timeSinceGPSEpoch time.Duration
	var timeField time.Time

	for _, rxInfo := range rxPacket.RXInfoSet {
		if rxInfo.TimeSinceGpsEpoch != nil {
			timeSinceGPSEpoch, err = ptypes.Duration(rxInfo.TimeSinceGpsEpoch)
			if err != nil {
				log.WithError(err).Error("time since gps epoch to duration error")
				continue
			}
		} else if rxInfo.Time != nil {
			timeField, err = ptypes.Timestamp(rxInfo.Time)
			if err != nil {
				log.WithError(err).Error("time to timestamp error")
				continue
			}
		}
	}

	log.WithFields(log.Fields{
		"dev_eui": ds.DevEUI,
	}).Info("device_time_req received")

	// fallback on time field when time since GPS epoch is not available
	if timeSinceGPSEpoch == 0 {

		// fallback on current server time when time field is not available
		if timeField.IsZero() {
			timeField = time.Now()
		}

		timeSinceGPSEpoch = gps.Time(timeField).TimeSinceGPSEpoch()
	}

	return []storage.MACCommandBlock{
		{
			CID: lorawan.DeviceTimeAns,
			MACCommands: storage.MACCommands{
				{
					CID: lorawan.DeviceTimeAns,
					Payload: &lorawan.DeviceTimeAnsPayload{
						TimeSinceGPSEpoch: timeSinceGPSEpoch,
					},
				},
			},
		},
	}, nil
}
