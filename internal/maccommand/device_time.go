package maccommand

import (
	"time"

	"github.com/golang/protobuf/ptypes"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/loraserver/internal/models"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/lorawan"
)

func handleDeviceTimeReq(ds *storage.DeviceSession, rxPacket models.RXPacket) ([]storage.MACCommandBlock, error) {
	if len(rxPacket.RXInfoSet) == 0 {
		return nil, errors.New("rx info-set contains zero items")
	}

	var err error
	var timeSinceGPSEpoch time.Duration

	for _, rxInfo := range rxPacket.RXInfoSet {
		if rxInfo.TimeSinceGpsEpoch != nil {
			timeSinceGPSEpoch, err = ptypes.Duration(rxInfo.TimeSinceGpsEpoch)
			if err != nil {
				log.WithError(err).Error("time since gps epoch to duration error")
				continue
			}
		}
	}

	log.WithFields(log.Fields{
		"dev_eui": ds.DevEUI,
	}).Info("device_time_req received")

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
