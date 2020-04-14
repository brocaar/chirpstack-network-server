package stats

import (
	"context"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/chirpstack-api/go/v3/as"
	"github.com/brocaar/chirpstack-api/go/v3/common"
	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/brocaar/chirpstack-network-server/internal/backend/gateway"
	"github.com/brocaar/chirpstack-network-server/internal/band"
	"github.com/brocaar/chirpstack-network-server/internal/helpers"
	"github.com/brocaar/chirpstack-network-server/internal/logging"
	"github.com/brocaar/chirpstack-network-server/internal/storage"
	loraband "github.com/brocaar/lorawan/band"
)

type statsContext struct {
	ctx          context.Context
	gateway      storage.Gateway
	gatewayStats gw.GatewayStats
}

var tasks = []func(*statsContext) error{
	getGateway,
	updateGatewayState,
	handleGatewayConfigurationUpdate,
	forwardGatewayStats,
}

// Handle handles the gateway stats
func Handle(ctx context.Context, stats gw.GatewayStats) error {
	sctx := statsContext{
		ctx:          ctx,
		gatewayStats: stats,
	}

	for _, t := range tasks {
		if err := t(&sctx); err != nil {
			return err
		}
	}

	return nil
}

func getGateway(ctx *statsContext) error {
	gatewayID := helpers.GetGatewayID(&ctx.gatewayStats)
	gw, err := storage.GetAndCacheGateway(ctx.ctx, storage.DB(), gatewayID)
	if err != nil {
		return errors.Wrap(err, "get gateway error")
	}

	ctx.gateway = gw
	return nil
}

func updateGatewayState(ctx *statsContext) error {
	now := time.Now()
	if ctx.gateway.FirstSeenAt == nil {
		ctx.gateway.FirstSeenAt = &now
	}
	ctx.gateway.LastSeenAt = &now

	if ctx.gatewayStats.Location != nil {
		ctx.gateway.Location.Latitude = ctx.gatewayStats.Location.Latitude
		ctx.gateway.Location.Longitude = ctx.gatewayStats.Location.Longitude
		ctx.gateway.Altitude = ctx.gatewayStats.Location.Altitude
	}

	if err := storage.UpdateGateway(ctx.ctx, storage.DB(), &ctx.gateway); err != nil {
		return errors.Wrap(err, "update gateway error")
	}

	if err := storage.FlushGatewayCache(ctx.ctx, ctx.gateway.GatewayID); err != nil {
		return errors.Wrap(err, "flush gateway cache error")
	}

	return nil
}

func handleGatewayConfigurationUpdate(ctx *statsContext) error {
	if ctx.gateway.GatewayProfileID == nil {
		log.WithFields(log.Fields{
			"gateway_id": ctx.gateway.GatewayID,
			"ctx_id":     ctx.ctx.Value(logging.ContextIDKey),
		}).Debug("gateway-profile is not set, skipping configuration update")
		return nil
	}

	gwProfile, err := storage.GetGatewayProfile(ctx.ctx, storage.DB(), *ctx.gateway.GatewayProfileID)
	if err != nil {
		return errors.Wrap(err, "get gateway-profile error")
	}

	if gwProfile.GetVersion() == ctx.gatewayStats.ConfigVersion || gwProfile.GetVersion() == ctx.gatewayStats.GetMetaData()["config_version"] {
		log.WithFields(log.Fields{
			"gateway_id": ctx.gateway.GatewayID,
			"version":    ctx.gatewayStats.ConfigVersion,
			"ctx_id":     ctx.ctx.Value(logging.ContextIDKey),
		}).Debug("gateway configuration is up-to-date")
		return nil
	}

	configPacket := gw.GatewayConfiguration{
		GatewayId: ctx.gateway.GatewayID[:],
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

func forwardGatewayStats(ctx *statsContext) error {
	rp, err := storage.GetRoutingProfile(ctx.ctx, storage.DB(), ctx.gateway.RoutingProfileID)
	if err != nil {
		return errors.Wrap(err, "get routing-profile error")
	}

	asClient, err := rp.GetApplicationServerClient()
	if err != nil {
		return errors.Wrap(err, "get application-server client error")
	}

	_, err = asClient.HandleGatewayStats(ctx.ctx, &as.HandleGatewayStatsRequest{
		GatewayId:           ctx.gatewayStats.GatewayId,
		StatsId:             ctx.gatewayStats.StatsId,
		Time:                ctx.gatewayStats.Time,
		Location:            ctx.gatewayStats.Location,
		RxPacketsReceived:   ctx.gatewayStats.RxPacketsReceived,
		RxPacketsReceivedOk: ctx.gatewayStats.RxPacketsReceivedOk,
		TxPacketsReceived:   ctx.gatewayStats.TxPacketsReceived,
		TxPacketsEmitted:    ctx.gatewayStats.TxPacketsEmitted,
		Metadata:            ctx.gatewayStats.MetaData,
	})
	if err != nil {
		return errors.Wrap(err, "handle gateway stats error")
	}

	return nil
}
