package api

import (
	"time"

	log "github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/internal/api/auth"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/gateway"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/band"
)

// GatewayAPI defines the gateway API.
type GatewayAPI struct {
	validator auth.Validator
}

// NewGatewayAPI returns a new GatewayAPI.
func NewGatewayAPI(validator auth.Validator) *GatewayAPI {
	return &GatewayAPI{
		validator: validator,
	}
}

// GetConfiguration returns the gateway configuration for the given MAC.
func (a *GatewayAPI) GetConfiguration(ctx context.Context, req *gw.GetConfigurationRequest) (*gw.GetConfigurationResponse, error) {
	mac, err := a.validator.GetMAC(ctx)
	if err != nil {
		return nil, grpc.Errorf(codes.Unauthenticated, "authentication failed: %s", err)
	}

	var reqMAC lorawan.EUI64
	copy(reqMAC[:], req.Mac)

	if mac != reqMAC {
		return nil, grpc.Errorf(codes.InvalidArgument, "invalid mac")
	}

	g, err := gateway.GetGateway(config.C.PostgreSQL.DB, mac)
	if err != nil {
		return nil, errToRPCError(err)
	}

	if g.ChannelConfigurationID == nil {
		log.WithField("mac", g.MAC).Warning("gateway configuration requested but gateway does not have channel-configuration")
		return &gw.GetConfigurationResponse{}, nil
	}

	channelConf, err := gateway.GetChannelConfiguration(config.C.PostgreSQL.DB, *g.ChannelConfigurationID)
	if err != nil {
		return nil, errToRPCError(err)
	}

	extraChannels, err := gateway.GetExtraChannelsForChannelConfigurationID(config.C.PostgreSQL.DB, *g.ChannelConfigurationID)
	if err != nil {
		return nil, errToRPCError(err)
	}

	out := gw.GetConfigurationResponse{
		UpdatedAt: channelConf.UpdatedAt.Format(time.RFC3339Nano),
	}

	for _, cidx := range channelConf.Channels {
		if int(cidx) >= len(config.C.NetworkServer.Band.Band.UplinkChannels) {
			log.WithFields(log.Fields{
				"mac": g.MAC,
				"channel_configuration_id": g.ChannelConfigurationID,
				"invalid_channel":          cidx,
			}).Error("channel-configuration channel does not exist in band")
			return nil, grpc.Errorf(codes.Internal, "invalid channel configuration")
		}

		channel := config.C.NetworkServer.Band.Band.UplinkChannels[int(cidx)]
		gwChannel := gw.Channel{
			Frequency: int32(channel.Frequency),
		}

		for dr := channel.MinDR; dr <= channel.MaxDR; dr++ {
			gwChannel.Bandwidth = int32(config.C.NetworkServer.Band.Band.DataRates[dr].Bandwidth)

			switch config.C.NetworkServer.Band.Band.DataRates[dr].Modulation {
			case band.LoRaModulation:
				gwChannel.Modulation = gw.Modulation_LORA
				gwChannel.SpreadFactors = append(gwChannel.SpreadFactors, int32(config.C.NetworkServer.Band.Band.DataRates[dr].SpreadFactor))
			case band.FSKModulation:
				gwChannel.Modulation = gw.Modulation_FSK
				gwChannel.BitRate = int32(config.C.NetworkServer.Band.Band.DataRates[dr].BitRate)
			}
		}

		out.Channels = append(out.Channels, &gwChannel)
	}

	for _, ch := range extraChannels {
		gwChannel := gw.Channel{
			Frequency: int32(ch.Frequency),
			Bandwidth: int32(ch.BandWidth),
		}

		switch band.Modulation(ch.Modulation) {
		case band.LoRaModulation:
			gwChannel.Modulation = gw.Modulation_LORA
		case band.FSKModulation:
			gwChannel.Modulation = gw.Modulation_FSK
			gwChannel.BitRate = int32(ch.BitRate)
		}

		for _, sf := range ch.SpreadFactors {
			gwChannel.SpreadFactors = append(gwChannel.SpreadFactors, int32(sf))
		}

		out.Channels = append(out.Channels, &gwChannel)
	}

	return &out, nil
}
