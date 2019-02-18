package gateway

import (
	"crypto/aes"
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/gomodule/redigo/redis"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/loraserver/api/common"
	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/backend/gateway"
	"github.com/brocaar/loraserver/internal/band"
	"github.com/brocaar/loraserver/internal/helpers"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/lorawan"
	loraband "github.com/brocaar/lorawan/band"
)

// StatsHandler represents a stat handler for incoming gateway stats.
type StatsHandler struct {
	wg sync.WaitGroup
}

// NewStatsHandler creates a new StatsHandler.
func NewStatsHandler() *StatsHandler {
	return &StatsHandler{}
}

// Start starts the stats handler.
func (s *StatsHandler) Start() error {
	go func() {
		s.wg.Add(1)
		defer s.wg.Done()

		for stats := range gateway.Backend().StatsPacketChan() {
			go func(stats gw.GatewayStats) {
				s.wg.Add(1)
				defer s.wg.Done()

				if err := updateGatewayState(storage.DB(), storage.RedisPool(), stats); err != nil {
					log.WithError(err).Error("update gateway state error")
				}

				if err := handleGatewayStats(storage.RedisPool(), stats); err != nil {
					log.WithError(err).Error("handle gateway stats error")
				}
			}(stats)
		}
	}()
	return nil
}

// Stop waits for the stats handler to complete the pending packets.
// At this stage the gateway backend must already been closed.
func (s *StatsHandler) Stop() error {
	s.wg.Wait()
	return nil
}

func handleGatewayStats(p *redis.Pool, stats gw.GatewayStats) error {
	gatewayID := helpers.GetGatewayID(&stats)

	ts, err := ptypes.Timestamp(stats.Time)
	if err != nil {
		return errors.Wrap(err, "timestamp error")
	}

	metrics := storage.MetricsRecord{
		Time: ts,
		Metrics: map[string]float64{
			"rx_count":    float64(stats.RxPacketsReceived),
			"rx_ok_count": float64(stats.RxPacketsReceivedOk),
			"tx_count":    float64(stats.TxPacketsReceived),
			"tx_ok_count": float64(stats.TxPacketsEmitted),
		},
	}

	err = storage.SaveMetrics(p, "gw:"+gatewayID.String(), metrics)
	if err != nil {
		return errors.Wrap(err, "save metrics error")
	}

	return nil
}

func updateGatewayState(db sqlx.Ext, p *redis.Pool, stats gw.GatewayStats) error {
	gatewayID := helpers.GetGatewayID(&stats)
	gw, err := storage.GetAndCacheGateway(db, p, gatewayID)
	if err != nil {
		return errors.Wrap(err, "get gateway error")
	}

	now := time.Now()

	if gw.FirstSeenAt == nil {
		gw.FirstSeenAt = &now
	}
	gw.LastSeenAt = &now

	if stats.Location != nil {
		gw.Location.Latitude = stats.Location.Latitude
		gw.Location.Longitude = stats.Location.Longitude
		gw.Altitude = stats.Location.Altitude
	}

	if err := storage.UpdateGateway(db, &gw); err != nil {
		return errors.Wrap(err, "update gateway error")
	}

	if err := storage.FlushGatewayCache(p, gatewayID); err != nil {
		return errors.Wrap(err, "flush gateway cache error")
	}

	if err := handleConfigurationUpdate(db, gw, stats.ConfigVersion); err != nil {
		return errors.Wrap(err, "handle gateway stats error")
	}

	return nil
}

// UpdateMetaDataInRxInfoSet updates the gateway meta-data in the
// given rx-info set. It will:
//   - add the gateway location
//   - set the FPGA id if available
//   - decrypt the fine-timestamp (if available and AES key is set)
func UpdateMetaDataInRxInfoSet(db sqlx.Queryer, p *redis.Pool, rxInfo []*gw.UplinkRXInfo) error {
	for i := range rxInfo {
		id := helpers.GetGatewayID(rxInfo[i])
		g, err := storage.GetAndCacheGateway(db, p, id)
		if err != nil {
			log.WithFields(log.Fields{
				"gateway_id": id,
			}).WithError(err).Error("get gateway error")
			continue
		}

		// set gateway location
		rxInfo[i].Location = &common.Location{
			Latitude:  g.Location.Latitude,
			Longitude: g.Location.Longitude,
			Altitude:  g.Altitude,
		}

		var board storage.GatewayBoard
		if int(rxInfo[i].Board) < len(g.Boards) {
			board = g.Boards[int(rxInfo[i].Board)]
		}

		// set FPGA ID
		// this is useful when the AES decryption key is not set as it
		// indicates which key to use for decryption
		if rxInfo[i].FineTimestampType == gw.FineTimestampType_ENCRYPTED && board.FPGAID != nil {
			tsInfo := rxInfo[i].GetEncryptedFineTimestamp()
			if tsInfo == nil {
				log.WithFields(log.Fields{
					"gateway_id": id,
				}).Error("encrypted_fine_timestamp must not be nil")
				continue
			}

			if len(tsInfo.FpgaId) == 0 {
				tsInfo.FpgaId = board.FPGAID[:]
			}
		}

		// decrypt fine-timestamp when the AES key is known
		if rxInfo[i].FineTimestampType == gw.FineTimestampType_ENCRYPTED && board.FineTimestampKey != nil {
			tsInfo := rxInfo[i].GetEncryptedFineTimestamp()
			if tsInfo == nil {
				log.WithFields(log.Fields{
					"gateway_id": id,
				}).Error("encrypted_fine_timestamp must not be nil")
				continue
			}

			if rxInfo[i].Time == nil {
				log.WithFields(log.Fields{
					"gateway_id": id,
				}).Error("time must not be nil")
				continue
			}

			rxTime, err := ptypes.Timestamp(rxInfo[i].Time)
			if err != nil {
				log.WithFields(log.Fields{
					"gateway_id": id,
				}).WithError(err).Error("get timestamp error")
			}

			plainTS, err := decryptFineTimestamp(*board.FineTimestampKey, rxTime, *tsInfo)
			if err != nil {
				log.WithFields(log.Fields{
					"gateway_id": id,
				}).WithError(err).Error("decrypt fine-timestamp error")
				continue
			}

			rxInfo[i].FineTimestampType = gw.FineTimestampType_PLAIN
			rxInfo[i].FineTimestamp = &gw.UplinkRXInfo_PlainFineTimestamp{
				PlainFineTimestamp: &plainTS,
			}
		}
	}

	return nil
}

func decryptFineTimestamp(key lorawan.AES128Key, rxTime time.Time, ts gw.EncryptedFineTimestamp) (gw.PlainFineTimestamp, error) {
	var plainTS gw.PlainFineTimestamp

	block, err := aes.NewCipher(key[:])
	if err != nil {
		return plainTS, errors.Wrap(err, "new cipher error")
	}

	if len(ts.EncryptedNs) != block.BlockSize() {
		return plainTS, fmt.Errorf("invalid block-size (%d) or ciphertext length (%d)", block.BlockSize(), len(ts.EncryptedNs))
	}

	ct := make([]byte, block.BlockSize())
	block.Decrypt(ct, ts.EncryptedNs)

	nanoSec := binary.BigEndian.Uint64(ct[len(ct)-8:])
	nanoSec = nanoSec / 32

	if time.Duration(nanoSec) >= time.Second {
		return plainTS, errors.New("expected fine-timestamp nanosecond remainder must be < 1 second, did you set the correct decryption key?")
	}

	rxTime = rxTime.Add(time.Duration(nanoSec) * time.Nanosecond)

	plainTS.Time, err = ptypes.TimestampProto(rxTime)
	if err != nil {
		return plainTS, errors.Wrap(err, "timestamp proto error")
	}

	return plainTS, nil
}

func handleConfigurationUpdate(db sqlx.Queryer, g storage.Gateway, currentVersion string) error {
	if g.GatewayProfileID == nil {
		log.WithField("gateway_id", g.GatewayID).Debug("gateway-profile is not set, skipping configuration update")
		return nil
	}

	gwProfile, err := storage.GetGatewayProfile(db, *g.GatewayProfileID)
	if err != nil {
		return errors.Wrap(err, "get gateway-profile error")
	}

	if gwProfile.GetVersion() == currentVersion {
		log.WithFields(log.Fields{
			"gateway_id": g.GatewayID,
			"version":    currentVersion,
		}).Debug("gateway configuration is up-to-date")
		return nil
	}

	configPacket := gw.GatewayConfiguration{
		GatewayId: g.GatewayID[:],
		Version:   gwProfile.GetVersion(),
	}

	for _, i := range gwProfile.Channels {
		c, err := band.Band().GetUplinkChannel(int(i))
		if err != nil {
			return errors.Wrap(err, "get channel error")
		}

		gwC := gw.ChannelConfiguration{
			Frequency:  uint32(c.Frequency),
			Modulation: common.Modulation_LORA,
		}

		modConfig := gw.LoRaModulationConfig{}

		for drI := c.MaxDR; drI >= c.MinDR; drI-- {
			dr, err := band.Band().GetDataRate(drI)
			if err != nil {
				return errors.Wrap(err, "get data-rate error")
			}

			modConfig.SpreadingFactors = append(modConfig.SpreadingFactors, uint32(dr.SpreadFactor))
			modConfig.Bandwidth = uint32(dr.Bandwidth)
		}

		gwC.ModulationConfig = &gw.ChannelConfiguration_LoraModulationConfig{
			LoraModulationConfig: &modConfig,
		}

		configPacket.Channels = append(configPacket.Channels, &gwC)
	}

	for _, c := range gwProfile.ExtraChannels {
		gwC := gw.ChannelConfiguration{
			Frequency: uint32(c.Frequency),
		}

		switch loraband.Modulation(c.Modulation) {
		case loraband.LoRaModulation:
			gwC.Modulation = common.Modulation_LORA
			modConfig := gw.LoRaModulationConfig{
				Bandwidth: uint32(c.Bandwidth),
			}

			for _, sf := range c.SpreadingFactors {
				modConfig.SpreadingFactors = append(modConfig.SpreadingFactors, uint32(sf))
			}

			gwC.ModulationConfig = &gw.ChannelConfiguration_LoraModulationConfig{
				LoraModulationConfig: &modConfig,
			}
		case loraband.FSKModulation:
			gwC.Modulation = common.Modulation_FSK
			modConfig := gw.FSKModulationConfig{
				Bandwidth: uint32(c.Bandwidth),
				Bitrate:   uint32(c.Bitrate),
			}

			gwC.ModulationConfig = &gw.ChannelConfiguration_FskModulationConfig{
				FskModulationConfig: &modConfig,
			}
		}

		configPacket.Channels = append(configPacket.Channels, &gwC)
	}

	if err := gateway.Backend().SendGatewayConfigPacket(configPacket); err != nil {
		return errors.Wrap(err, "send gateway-configuration packet error")
	}

	return nil
}
