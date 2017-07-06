package api

import (
	"encoding/json"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/api/ns"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/downlink"
	"github.com/brocaar/loraserver/internal/gateway"
	"github.com/brocaar/loraserver/internal/maccommand"
	"github.com/brocaar/loraserver/internal/node"
	"github.com/brocaar/loraserver/internal/session"
	"github.com/brocaar/lorawan"
)

// defaultCodeRate defines the default code rate
const defaultCodeRate = "4/5"

// NetworkServerAPI defines the nework-server API.
type NetworkServerAPI struct {
	ctx common.Context
}

// NewNetworkServerAPI returns a new NetworkServerAPI.
func NewNetworkServerAPI(ctx common.Context) *NetworkServerAPI {
	return &NetworkServerAPI{
		ctx: ctx,
	}
}

// CreateNodeSession create a node-session.
func (n *NetworkServerAPI) CreateNodeSession(ctx context.Context, req *ns.CreateNodeSessionRequest) (*ns.CreateNodeSessionResponse, error) {
	sess := session.NodeSession{
		FCntUp:             req.FCntUp,
		FCntDown:           req.FCntDown,
		RXDelay:            uint8(req.RxDelay),
		RX1DROffset:        uint8(req.Rx1DROffset),
		RXWindow:           session.RXWindow(req.RxWindow),
		RX2DR:              uint8(req.Rx2DR),
		RelaxFCnt:          req.RelaxFCnt,
		ADRInterval:        req.AdrInterval,
		InstallationMargin: req.InstallationMargin,
		EnabledChannels:    common.Band.GetUplinkChannels(),
	}

	copy(sess.DevAddr[:], req.DevAddr)
	copy(sess.AppEUI[:], req.AppEUI)
	copy(sess.DevEUI[:], req.DevEUI)
	copy(sess.NwkSKey[:], req.NwkSKey)

	exists, err := session.NodeSessionExists(n.ctx.RedisPool, sess.DevEUI)
	if err != nil {
		return nil, errToRPCError(err)
	}
	if exists {
		return nil, grpc.Errorf(codes.AlreadyExists, "node-session already exists")
	}

	if err := session.SaveNodeSession(n.ctx.RedisPool, sess); err != nil {
		return nil, errToRPCError(err)
	}

	if err := maccommand.FlushQueue(n.ctx.RedisPool, sess.DevEUI); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.CreateNodeSessionResponse{}, nil
}

// GetNodeSession returns a node-session.
func (n *NetworkServerAPI) GetNodeSession(ctx context.Context, req *ns.GetNodeSessionRequest) (*ns.GetNodeSessionResponse, error) {
	var devEUI lorawan.EUI64
	copy(devEUI[:], req.DevEUI)

	sess, err := session.GetNodeSession(n.ctx.RedisPool, devEUI)
	if err != nil {
		return nil, errToRPCError(err)
	}

	resp := &ns.GetNodeSessionResponse{
		DevAddr:            sess.DevAddr[:],
		AppEUI:             sess.AppEUI[:],
		DevEUI:             sess.DevEUI[:],
		NwkSKey:            sess.NwkSKey[:],
		FCntUp:             sess.FCntUp,
		FCntDown:           sess.FCntDown,
		RxDelay:            uint32(sess.RXDelay),
		Rx1DROffset:        uint32(sess.RX1DROffset),
		RxWindow:           ns.RXWindow(sess.RXWindow),
		Rx2DR:              uint32(sess.RX2DR),
		RelaxFCnt:          sess.RelaxFCnt,
		AdrInterval:        sess.ADRInterval,
		InstallationMargin: sess.InstallationMargin,
		NbTrans:            uint32(sess.NbTrans),
		TxPowerIndex:       uint32(sess.TXPowerIndex),
	}

	return resp, nil
}

// UpdateNodeSession updates a node-session.
func (n *NetworkServerAPI) UpdateNodeSession(ctx context.Context, req *ns.UpdateNodeSessionRequest) (*ns.UpdateNodeSessionResponse, error) {
	var devAddr lorawan.DevAddr
	copy(devAddr[:], req.DevAddr)

	var devEUI, appEUI lorawan.EUI64
	copy(devEUI[:], req.DevEUI)
	copy(appEUI[:], req.AppEUI)

	sess, err := session.GetNodeSession(n.ctx.RedisPool, devEUI)
	if err != nil {
		return nil, errToRPCError(err)
	}

	if sess.AppEUI != appEUI {
		return nil, grpc.Errorf(codes.InvalidArgument, "node-session belongs to a different AppEUI")
	}

	newSess := session.NodeSession{
		FCntUp:             req.FCntUp,
		FCntDown:           req.FCntDown,
		RXDelay:            uint8(req.RxDelay),
		RX1DROffset:        uint8(req.Rx1DROffset),
		RXWindow:           session.RXWindow(req.RxWindow),
		RX2DR:              uint8(req.Rx2DR),
		RelaxFCnt:          req.RelaxFCnt,
		ADRInterval:        req.AdrInterval,
		InstallationMargin: req.InstallationMargin,

		// these values can't be overwritten
		NbTrans:         sess.NbTrans,
		TXPowerIndex:    sess.TXPowerIndex,
		UplinkHistory:   sess.UplinkHistory,
		EnabledChannels: sess.EnabledChannels,
	}

	copy(newSess.DevAddr[:], req.DevAddr)
	copy(newSess.AppEUI[:], req.AppEUI)
	copy(newSess.DevEUI[:], req.DevEUI)
	copy(newSess.NwkSKey[:], req.NwkSKey)

	if err := session.SaveNodeSession(n.ctx.RedisPool, newSess); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.UpdateNodeSessionResponse{}, nil
}

// DeleteNodeSession deletes a node-session.
func (n *NetworkServerAPI) DeleteNodeSession(ctx context.Context, req *ns.DeleteNodeSessionRequest) (*ns.DeleteNodeSessionResponse, error) {
	var devEUI lorawan.EUI64
	copy(devEUI[:], req.DevEUI)

	if err := session.DeleteNodeSession(n.ctx.RedisPool, devEUI); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.DeleteNodeSessionResponse{}, nil
}

// GetRandomDevAddr returns a random DevAddr.
func (n *NetworkServerAPI) GetRandomDevAddr(ctx context.Context, req *ns.GetRandomDevAddrRequest) (*ns.GetRandomDevAddrResponse, error) {
	devAddr, err := session.GetRandomDevAddr(n.ctx.RedisPool, n.ctx.NetID)
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.GetRandomDevAddrResponse{
		DevAddr: devAddr[:],
	}, nil
}

// EnqueueDataDownMACCommand adds a data down MAC command to the queue.
// It replaces already enqueued mac-commands with the same CID.
func (n *NetworkServerAPI) EnqueueDataDownMACCommand(ctx context.Context, req *ns.EnqueueDataDownMACCommandRequest) (*ns.EnqueueDataDownMACCommandResponse, error) {
	var commands []lorawan.MACCommand
	var devEUI lorawan.EUI64

	copy(devEUI[:], req.DevEUI)

	for _, b := range req.Commands {
		var mac lorawan.MACCommand
		if err := mac.UnmarshalBinary(false, b); err != nil {
			return nil, grpc.Errorf(codes.InvalidArgument, err.Error())
		}
		commands = append(commands, mac)
	}

	block := maccommand.Block{
		CID:         lorawan.CID(req.Cid),
		FRMPayload:  req.FrmPayload,
		External:    true,
		MACCommands: commands,
	}

	if err := maccommand.AddQueueItem(n.ctx.RedisPool, devEUI, block); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.EnqueueDataDownMACCommandResponse{}, nil
}

// PushDataDown pushes the given downlink payload to the node (only works for Class-C nodes).
func (n *NetworkServerAPI) PushDataDown(ctx context.Context, req *ns.PushDataDownRequest) (*ns.PushDataDownResponse, error) {
	var devEUI lorawan.EUI64
	copy(devEUI[:], req.DevEUI)

	sess, err := session.GetNodeSession(n.ctx.RedisPool, devEUI)
	if err != nil {
		return nil, errToRPCError(err)
	}

	if req.FCnt != sess.FCntDown {
		return nil, grpc.Errorf(codes.InvalidArgument, "invalid FCnt (expected: %d)", sess.FCntDown)
	}

	err = downlink.HandlePushDataDown(n.ctx, sess, req.Confirmed, uint8(req.FPort), req.Data)
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.PushDataDownResponse{}, nil
}

// CreateGateway creates the given gateway.
func (n *NetworkServerAPI) CreateGateway(ctx context.Context, req *ns.CreateGatewayRequest) (*ns.CreateGatewayResponse, error) {
	var mac lorawan.EUI64
	copy(mac[:], req.Mac)

	var location *gateway.GPSPoint
	var altitude *float64

	if req.Latitude != 0 && req.Longitude != 0 {
		location = &gateway.GPSPoint{
			Latitude:  req.Latitude,
			Longitude: req.Longitude,
		}
	}

	if req.Altitude != 0 {
		altitude = &req.Altitude
	}

	gw := gateway.Gateway{
		MAC:         mac,
		Name:        req.Name,
		Description: req.Description,
		Location:    location,
		Altitude:    altitude,
	}
	if req.ChannelConfigurationID != 0 {
		gw.ChannelConfigurationID = &req.ChannelConfigurationID
	}

	err := gateway.CreateGateway(n.ctx.DB, &gw)
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.CreateGatewayResponse{}, nil
}

// GetGateway returns data for a particular gateway.
func (n *NetworkServerAPI) GetGateway(ctx context.Context, req *ns.GetGatewayRequest) (*ns.GetGatewayResponse, error) {
	var mac lorawan.EUI64
	copy(mac[:], req.Mac)

	gw, err := gateway.GetGateway(n.ctx.DB, mac)
	if err != nil {
		return nil, errToRPCError(err)
	}

	return gwToResp(gw), nil
}

// UpdateGateway updates an existing gateway.
func (n *NetworkServerAPI) UpdateGateway(ctx context.Context, req *ns.UpdateGatewayRequest) (*ns.UpdateGatewayResponse, error) {
	var mac lorawan.EUI64
	copy(mac[:], req.Mac)

	gw, err := gateway.GetGateway(n.ctx.DB, mac)
	if err != nil {
		return nil, errToRPCError(err)
	}

	var location *gateway.GPSPoint
	var altitude *float64

	if req.Latitude != 0 && req.Longitude != 0 {
		location = &gateway.GPSPoint{
			Latitude:  req.Latitude,
			Longitude: req.Longitude,
		}
	}

	if req.Altitude != 0 {
		altitude = &req.Altitude
	}

	if req.ChannelConfigurationID != 0 {
		gw.ChannelConfigurationID = &req.ChannelConfigurationID
	} else {
		gw.ChannelConfigurationID = nil
	}

	gw.Name = req.Name
	gw.Description = req.Description
	gw.Location = location
	gw.Altitude = altitude

	err = gateway.UpdateGateway(n.ctx.DB, &gw)
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.UpdateGatewayResponse{}, nil
}

// ListGateways returns the existing gateways.
func (n *NetworkServerAPI) ListGateways(ctx context.Context, req *ns.ListGatewayRequest) (*ns.ListGatewayResponse, error) {
	count, err := gateway.GetGatewayCount(n.ctx.DB)
	if err != nil {
		return nil, errToRPCError(err)
	}

	gws, err := gateway.GetGateways(n.ctx.DB, int(req.Limit), int(req.Offset))
	if err != nil {
		return nil, errToRPCError(err)
	}

	resp := ns.ListGatewayResponse{
		TotalCount: int32(count),
	}

	for _, gw := range gws {
		resp.Result = append(resp.Result, gwToResp(gw))
	}

	return &resp, nil
}

// DeleteGateway deletes a gateway.
func (n *NetworkServerAPI) DeleteGateway(ctx context.Context, req *ns.DeleteGatewayRequest) (*ns.DeleteGatewayResponse, error) {
	var mac lorawan.EUI64
	copy(mac[:], req.Mac)

	err := gateway.DeleteGateway(n.ctx.DB, mac)
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.DeleteGatewayResponse{}, nil
}

// GetGatewayStats returns stats of an existing gateway.
func (n *NetworkServerAPI) GetGatewayStats(ctx context.Context, req *ns.GetGatewayStatsRequest) (*ns.GetGatewayStatsResponse, error) {
	var mac lorawan.EUI64
	copy(mac[:], req.Mac)

	start, err := time.Parse(time.RFC3339Nano, req.StartTimestamp)
	if err != nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "parse start timestamp: %s", err)
	}

	end, err := time.Parse(time.RFC3339Nano, req.EndTimestamp)
	if err != nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "parse end timestamp: %s", err)
	}

	stats, err := gateway.GetGatewayStats(n.ctx.DB, mac, req.Interval.String(), start, end)
	if err != nil {
		return nil, errToRPCError(err)
	}

	var resp ns.GetGatewayStatsResponse

	for _, stat := range stats {
		resp.Result = append(resp.Result, &ns.GatewayStats{
			Timestamp:           stat.Timestamp.Format(time.RFC3339Nano),
			RxPacketsReceived:   int32(stat.RXPacketsReceived),
			RxPacketsReceivedOK: int32(stat.RXPacketsReceivedOK),
			TxPacketsReceived:   int32(stat.TXPacketsReceived),
			TxPacketsEmitted:    int32(stat.TXPacketsEmitted),
		})
	}

	return &resp, nil
}

// GetFrameLogsForDevEUI returns the uplink / downlink frame logs for the given DevEUI.
func (n *NetworkServerAPI) GetFrameLogsForDevEUI(ctx context.Context, req *ns.GetFrameLogsForDevEUIRequest) (*ns.GetFrameLogsResponse, error) {
	var devEUI lorawan.EUI64
	copy(devEUI[:], req.DevEUI)

	count, err := node.GetFrameLogCountForDevEUI(n.ctx.DB, devEUI)
	if err != nil {
		return nil, errToRPCError(err)
	}

	logs, err := node.GetFrameLogsForDevEUI(n.ctx.DB, devEUI, int(req.Limit), int(req.Offset))
	if err != nil {
		return nil, errToRPCError(err)
	}

	resp := ns.GetFrameLogsResponse{
		TotalCount: int32(count),
	}

	for i := range logs {
		fl := ns.FrameLog{
			CreatedAt:  logs[i].CreatedAt.Format(time.RFC3339Nano),
			PhyPayload: logs[i].PHYPayload,
		}

		if txInfoJSON := logs[i].TXInfo; txInfoJSON != nil {
			var txInfo gw.TXInfo
			if err := json.Unmarshal(*txInfoJSON, &txInfo); err != nil {
				return nil, errToRPCError(err)
			}

			fl.TxInfo = &ns.TXInfo{
				CodeRate:    txInfo.CodeRate,
				Frequency:   int64(txInfo.Frequency),
				Immediately: txInfo.Immediately,
				Mac:         txInfo.MAC[:],
				Power:       int32(txInfo.Power),
				Timestamp:   txInfo.Timestamp,
				DataRate: &ns.DataRate{
					Modulation:   string(txInfo.DataRate.Modulation),
					BandWidth:    uint32(txInfo.DataRate.Bandwidth),
					SpreadFactor: uint32(txInfo.DataRate.SpreadFactor),
					Bitrate:      uint32(txInfo.DataRate.BitRate),
				},
			}
		}

		if rxInfoSetJSON := logs[i].RXInfoSet; rxInfoSetJSON != nil {
			var rxInfoSet []gw.RXInfo
			if err := json.Unmarshal(*rxInfoSetJSON, &rxInfoSet); err != nil {
				return nil, errToRPCError(err)
			}

			for i := range rxInfoSet {
				rxInfo := ns.RXInfo{
					Channel:   int32(rxInfoSet[i].Channel),
					CodeRate:  rxInfoSet[i].CodeRate,
					Frequency: int64(rxInfoSet[i].Frequency),
					LoRaSNR:   rxInfoSet[i].LoRaSNR,
					Rssi:      int32(rxInfoSet[i].RSSI),
					Time:      rxInfoSet[i].Time.Format(time.RFC3339Nano),
					Timestamp: rxInfoSet[i].Timestamp,
					DataRate: &ns.DataRate{
						Modulation:   string(rxInfoSet[i].DataRate.Modulation),
						BandWidth:    uint32(rxInfoSet[i].DataRate.Bandwidth),
						SpreadFactor: uint32(rxInfoSet[i].DataRate.SpreadFactor),
						Bitrate:      uint32(rxInfoSet[i].DataRate.BitRate),
					},
					Mac: rxInfoSet[i].MAC[:],
				}
				fl.RxInfoSet = append(fl.RxInfoSet, &rxInfo)
			}
		}

		resp.Result = append(resp.Result, &fl)
	}

	return &resp, nil
}

// CreateChannelConfiguration creates the given channel-configuration.
func (n *NetworkServerAPI) CreateChannelConfiguration(ctx context.Context, req *ns.CreateChannelConfigurationRequest) (*ns.CreateChannelConfigurationResponse, error) {
	cf := gateway.ChannelConfiguration{
		Name: req.Name,
		Band: string(common.BandName),
	}
	for _, c := range req.Channels {
		cf.Channels = append(cf.Channels, int64(c))
	}

	if err := gateway.CreateChannelConfiguration(n.ctx.DB, &cf); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.CreateChannelConfigurationResponse{Id: cf.ID}, nil
}

// GetChannelConfiguration returns the channel-configuration for the given ID.
func (n *NetworkServerAPI) GetChannelConfiguration(ctx context.Context, req *ns.GetChannelConfigurationRequest) (*ns.GetChannelConfigurationResponse, error) {
	cf, err := gateway.GetChannelConfiguration(n.ctx.DB, req.Id)
	if err != nil {
		return nil, errToRPCError(err)
	}

	return channelConfigurationToResp(cf), nil
}

// UpdateChannelConfiguration updates the given channel-configuration.
func (n *NetworkServerAPI) UpdateChannelConfiguration(ctx context.Context, req *ns.UpdateChannelConfigurationRequest) (*ns.UpdateChannelConfigurationResponse, error) {
	cf, err := gateway.GetChannelConfiguration(n.ctx.DB, req.Id)
	if err != nil {
		return nil, errToRPCError(err)
	}

	cf.Name = req.Name
	cf.Channels = []int64{}
	for _, c := range req.Channels {
		cf.Channels = append(cf.Channels, int64(c))
	}

	if err = gateway.UpdateChannelConfiguration(n.ctx.DB, &cf); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.UpdateChannelConfigurationResponse{}, nil
}

// DeleteChannelConfiguration deletes the channel-configuration matching the
// given ID.
func (n *NetworkServerAPI) DeleteChannelConfiguration(ctx context.Context, req *ns.DeleteChannelConfigurationRequest) (*ns.DeleteChannelConfigurationResponse, error) {
	if err := gateway.DeleteChannelConfiguration(n.ctx.DB, req.Id); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.DeleteChannelConfigurationResponse{}, nil
}

// ListChannelConfigurations returns all channel-configurations.
func (n *NetworkServerAPI) ListChannelConfigurations(ctx context.Context, req *ns.ListChannelConfigurationsRequest) (*ns.ListChannelConfigurationsResponse, error) {
	cfs, err := gateway.GetChannelConfigurationsForBand(n.ctx.DB, string(common.BandName))
	if err != nil {
		return nil, errToRPCError(err)
	}

	var out ns.ListChannelConfigurationsResponse

	for _, cf := range cfs {
		out.Result = append(out.Result, channelConfigurationToResp(cf))
	}

	return &out, nil
}

// CreateExtraChannel creates the given extra channel.
func (n *NetworkServerAPI) CreateExtraChannel(ctx context.Context, req *ns.CreateExtraChannelRequest) (*ns.CreateExtraChannelResponse, error) {
	ec := gateway.ExtraChannel{
		ChannelConfigurationID: req.ChannelConfigurationID,
		Frequency:              int(req.Frequency),
		BandWidth:              int(req.BandWidth),
		DataRate:               int(req.DataRate),
	}

	switch req.Modulation {
	case ns.Modulation_LORA:
		ec.Modulation = gateway.ChannelModulationLoRa
	case ns.Modulation_FSK:
		ec.Modulation = gateway.ChannelModulationFSK
	default:
		return nil, grpc.Errorf(codes.InvalidArgument, "invalid modulation")
	}

	for _, sf := range req.SpreadFactors {
		ec.SpreadFactors = append(ec.SpreadFactors, int64(sf))
	}

	if err := gateway.CreateExtraChannel(n.ctx.DB, &ec); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.CreateExtraChannelResponse{Id: ec.ID}, nil
}

// UpdateExtraChannel updates the given extra channel.
func (n *NetworkServerAPI) UpdateExtraChannel(ctx context.Context, req *ns.UpdateExtraChannelRequest) (*ns.UpdateExtraChannelResponse, error) {
	ec, err := gateway.GetExtraChannel(n.ctx.DB, req.Id)
	if err != nil {
		return nil, errToRPCError(err)
	}

	ec.ChannelConfigurationID = req.ChannelConfigurationID
	ec.Frequency = int(req.Frequency)
	ec.BandWidth = int(req.BandWidth)
	ec.DataRate = int(req.DataRate)
	ec.SpreadFactors = []int64{}

	switch req.Modulation {
	case ns.Modulation_LORA:
		ec.Modulation = gateway.ChannelModulationLoRa
	case ns.Modulation_FSK:
		ec.Modulation = gateway.ChannelModulationFSK
	default:
		return nil, grpc.Errorf(codes.InvalidArgument, "invalid modulation")
	}

	for _, sf := range req.SpreadFactors {
		ec.SpreadFactors = append(ec.SpreadFactors, int64(sf))
	}

	if err = gateway.UpdateExtraChannel(n.ctx.DB, &ec); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.UpdateExtraChannelResponse{}, nil
}

// DeleteExtraChannel deletes the extra channel matching the given id.
func (n *NetworkServerAPI) DeleteExtraChannel(ctx context.Context, req *ns.DeleteExtraChannelRequest) (*ns.DeleteExtraChannelResponse, error) {
	err := gateway.DeleteExtraChannel(n.ctx.DB, req.Id)
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.DeleteExtraChannelResponse{}, nil
}

// GetExtraChannelsForChannelConfigurationID returns the extra channels for
// the given channel-configuration id.
func (n *NetworkServerAPI) GetExtraChannelsForChannelConfigurationID(ctx context.Context, req *ns.GetExtraChannelsForChannelConfigurationIDRequest) (*ns.GetExtraChannelsForChannelConfigurationIDResponse, error) {
	chans, err := gateway.GetExtraChannelsForChannelConfigurationID(n.ctx.DB, req.Id)
	if err != nil {
		return nil, errToRPCError(err)
	}

	var out ns.GetExtraChannelsForChannelConfigurationIDResponse

	for i, c := range chans {
		out.Result = append(out.Result, &ns.GetExtraChannelResponse{
			Id: c.ID,
			ChannelConfigurationID: c.ChannelConfigurationID,
			CreatedAt:              c.CreatedAt.Format(time.RFC3339Nano),
			UpdatedAt:              c.UpdatedAt.Format(time.RFC3339Nano),
			Frequency:              int32(c.Frequency),
			Bandwidth:              int32(c.BandWidth),
			DataRate:               int32(c.DataRate),
		})

		for _, sf := range c.SpreadFactors {
			out.Result[i].SpreadFactors = append(out.Result[i].SpreadFactors, int32(sf))
		}

		switch c.Modulation {
		case gateway.ChannelModulationLoRa:
			out.Result[i].Modulation = ns.Modulation_LORA
		case gateway.ChannelModulationFSK:
			out.Result[i].Modulation = ns.Modulation_FSK
		default:
			return nil, grpc.Errorf(codes.Internal, "invalid modulation")
		}
	}

	return &out, nil
}

func channelConfigurationToResp(cf gateway.ChannelConfiguration) *ns.GetChannelConfigurationResponse {
	out := ns.GetChannelConfigurationResponse{
		Id:        cf.ID,
		Name:      cf.Name,
		CreatedAt: cf.CreatedAt.Format(time.RFC3339Nano),
		UpdatedAt: cf.UpdatedAt.Format(time.RFC3339Nano),
	}
	for _, c := range cf.Channels {
		out.Channels = append(out.Channels, int32(c))
	}
	return &out
}

func gwToResp(gw gateway.Gateway) *ns.GetGatewayResponse {
	resp := ns.GetGatewayResponse{
		Mac:         gw.MAC[:],
		Name:        gw.Name,
		Description: gw.Description,
		CreatedAt:   gw.CreatedAt.Format(time.RFC3339Nano),
		UpdatedAt:   gw.UpdatedAt.Format(time.RFC3339Nano),
	}

	if gw.FirstSeenAt != nil {
		resp.FirstSeenAt = gw.FirstSeenAt.Format(time.RFC3339Nano)
	}

	if gw.LastSeenAt != nil {
		resp.LastSeenAt = gw.LastSeenAt.Format(time.RFC3339Nano)
	}

	if gw.Location != nil {
		resp.Latitude = gw.Location.Latitude
		resp.Longitude = gw.Location.Longitude
	}

	if gw.Altitude != nil {
		resp.Altitude = *gw.Altitude
	}

	if gw.ChannelConfigurationID != nil {
		resp.ChannelConfigurationID = *gw.ChannelConfigurationID
	}

	return &resp
}
