package api

import (
	"encoding/json"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/jmoiron/sqlx"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/brocaar/loraserver/api/gw"
	"github.com/brocaar/loraserver/api/ns"
	"github.com/brocaar/loraserver/internal/api/auth"
	"github.com/brocaar/loraserver/internal/common"
	"github.com/brocaar/loraserver/internal/downlink"
	"github.com/brocaar/loraserver/internal/gateway"
	"github.com/brocaar/loraserver/internal/maccommand"
	"github.com/brocaar/loraserver/internal/node"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/backend"
	"github.com/brocaar/lorawan/band"
)

var rfRegionMapping = map[band.Name]backend.RFRegion{
	band.AS_923:     backend.AS923,
	band.AU_915_928: backend.Australia915,
	band.CN_470_510: backend.China470,
	band.CN_779_787: backend.China779,
	band.EU_433:     backend.EU433,
	band.EU_863_870: backend.EU868,
	band.IN_865_867: backend.RFRegion("India865"),      // ? is not defined
	band.KR_920_923: backend.RFRegion("SouthKorea920"), // ? is not defined
	band.US_902_928: backend.US902,
}

// defaultCodeRate defines the default code rate
const defaultCodeRate = "4/5"

// NetworkServerAPI defines the nework-server API.
type NetworkServerAPI struct{}

// NewNetworkServerAPI returns a new NetworkServerAPI.
func NewNetworkServerAPI() *NetworkServerAPI {
	return &NetworkServerAPI{}
}

// CreateServiceProfile creates the given service-profile.
func (n *NetworkServerAPI) CreateServiceProfile(ctx context.Context, req *ns.CreateServiceProfileRequest) (*ns.CreateServiceProfileResponse, error) {
	sp := storage.ServiceProfile{
		ServiceProfile: backend.ServiceProfile{
			ServiceProfileID:       req.ServiceProfile.ServiceProfileID,
			ULRate:                 int(req.ServiceProfile.UlRate),
			ULBucketSize:           int(req.ServiceProfile.UlBucketSize),
			DLRate:                 int(req.ServiceProfile.DlRate),
			DLBucketSize:           int(req.ServiceProfile.DlBucketSize),
			AddGWMetadata:          req.ServiceProfile.AddGWMetadata,
			DevStatusReqFreq:       int(req.ServiceProfile.DevStatusReqFreq),
			ReportDevStatusBattery: req.ServiceProfile.ReportDevStatusBattery,
			ReportDevStatusMargin:  req.ServiceProfile.ReportDevStatusMargin,
			DRMin:          int(req.ServiceProfile.DrMin),
			DRMax:          int(req.ServiceProfile.DrMax),
			ChannelMask:    backend.HEXBytes(req.ServiceProfile.ChannelMask),
			PRAllowed:      req.ServiceProfile.PrAllowed,
			HRAllowed:      req.ServiceProfile.HrAllowed,
			RAAllowed:      req.ServiceProfile.RaAllowed,
			NwkGeoLoc:      req.ServiceProfile.NwkGeoLoc,
			TargetPER:      backend.Percentage(req.ServiceProfile.TargetPER),
			MinGWDiversity: int(req.ServiceProfile.MinGWDiversity),
		},
	}

	switch req.ServiceProfile.UlRatePolicy {
	case ns.RatePolicy_MARK:
		sp.ServiceProfile.ULRatePolicy = backend.Mark
	case ns.RatePolicy_DROP:
		sp.ServiceProfile.ULRatePolicy = backend.Drop
	}

	switch req.ServiceProfile.DlRatePolicy {
	case ns.RatePolicy_MARK:
		sp.ServiceProfile.DLRatePolicy = backend.Mark
	case ns.RatePolicy_DROP:
		sp.ServiceProfile.DLRatePolicy = backend.Drop
	}

	if err := storage.CreateServiceProfile(common.DB, &sp); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.CreateServiceProfileResponse{
		ServiceProfileID: sp.ServiceProfile.ServiceProfileID,
	}, nil
}

// GetServiceProfile returns the service-profile matching the given id.
func (n *NetworkServerAPI) GetServiceProfile(ctx context.Context, req *ns.GetServiceProfileRequest) (*ns.GetServiceProfileResponse, error) {
	sp, err := storage.GetServiceProfile(common.DB, req.ServiceProfileID)
	if err != nil {
		return nil, errToRPCError(err)
	}

	resp := ns.GetServiceProfileResponse{
		CreatedAt: sp.CreatedAt.Format(time.RFC3339Nano),
		UpdatedAt: sp.UpdatedAt.Format(time.RFC3339Nano),
		ServiceProfile: &ns.ServiceProfile{
			ServiceProfileID:       sp.ServiceProfile.ServiceProfileID,
			UlRate:                 uint32(sp.ServiceProfile.ULRate),
			UlBucketSize:           uint32(sp.ServiceProfile.ULBucketSize),
			DlRate:                 uint32(sp.ServiceProfile.DLRate),
			DlBucketSize:           uint32(sp.ServiceProfile.DLBucketSize),
			AddGWMetadata:          sp.ServiceProfile.AddGWMetadata,
			DevStatusReqFreq:       uint32(sp.ServiceProfile.DevStatusReqFreq),
			ReportDevStatusBattery: sp.ServiceProfile.ReportDevStatusBattery,
			ReportDevStatusMargin:  sp.ServiceProfile.ReportDevStatusMargin,
			DrMin:          uint32(sp.ServiceProfile.DRMin),
			DrMax:          uint32(sp.ServiceProfile.DRMax),
			ChannelMask:    []byte(sp.ServiceProfile.ChannelMask),
			PrAllowed:      sp.ServiceProfile.PRAllowed,
			HrAllowed:      sp.ServiceProfile.HRAllowed,
			RaAllowed:      sp.ServiceProfile.RAAllowed,
			NwkGeoLoc:      sp.ServiceProfile.NwkGeoLoc,
			TargetPER:      uint32(sp.ServiceProfile.TargetPER),
			MinGWDiversity: uint32(sp.ServiceProfile.MinGWDiversity),
		},
	}

	switch sp.ServiceProfile.ULRatePolicy {
	case backend.Mark:
		resp.ServiceProfile.UlRatePolicy = ns.RatePolicy_MARK
	case backend.Drop:
		resp.ServiceProfile.UlRatePolicy = ns.RatePolicy_DROP
	}

	switch sp.ServiceProfile.DLRatePolicy {
	case backend.Mark:
		resp.ServiceProfile.DlRatePolicy = ns.RatePolicy_MARK
	case backend.Drop:
		resp.ServiceProfile.DlRatePolicy = ns.RatePolicy_DROP
	}

	return &resp, nil
}

// UpdateServiceProfile updates the given service-profile.
func (n *NetworkServerAPI) UpdateServiceProfile(ctx context.Context, req *ns.UpdateServiceProfileRequest) (*ns.UpdateServiceProfileResponse, error) {
	sp, err := storage.GetServiceProfile(common.DB, req.ServiceProfile.ServiceProfileID)
	if err != nil {
		return nil, errToRPCError(err)
	}

	sp.ServiceProfile = backend.ServiceProfile{
		ServiceProfileID:       sp.ServiceProfile.ServiceProfileID,
		ULRate:                 int(req.ServiceProfile.UlRate),
		ULBucketSize:           int(req.ServiceProfile.UlBucketSize),
		DLRate:                 int(req.ServiceProfile.DlRate),
		DLBucketSize:           int(req.ServiceProfile.DlBucketSize),
		AddGWMetadata:          req.ServiceProfile.AddGWMetadata,
		DevStatusReqFreq:       int(req.ServiceProfile.DevStatusReqFreq),
		ReportDevStatusBattery: req.ServiceProfile.ReportDevStatusBattery,
		ReportDevStatusMargin:  req.ServiceProfile.ReportDevStatusMargin,
		DRMin:          int(req.ServiceProfile.DrMin),
		DRMax:          int(req.ServiceProfile.DrMax),
		ChannelMask:    backend.HEXBytes(req.ServiceProfile.ChannelMask),
		PRAllowed:      req.ServiceProfile.PrAllowed,
		HRAllowed:      req.ServiceProfile.HrAllowed,
		RAAllowed:      req.ServiceProfile.RaAllowed,
		NwkGeoLoc:      req.ServiceProfile.NwkGeoLoc,
		TargetPER:      backend.Percentage(req.ServiceProfile.TargetPER),
		MinGWDiversity: int(req.ServiceProfile.MinGWDiversity),
	}

	switch req.ServiceProfile.UlRatePolicy {
	case ns.RatePolicy_MARK:
		sp.ServiceProfile.ULRatePolicy = backend.Mark
	case ns.RatePolicy_DROP:
		sp.ServiceProfile.ULRatePolicy = backend.Drop
	}

	switch req.ServiceProfile.DlRatePolicy {
	case ns.RatePolicy_MARK:
		sp.ServiceProfile.DLRatePolicy = backend.Mark
	case ns.RatePolicy_DROP:
		sp.ServiceProfile.DLRatePolicy = backend.Drop
	}

	if err := storage.FlushServiceProfileCache(common.RedisPool, sp.ServiceProfile.ServiceProfileID); err != nil {
		return nil, errToRPCError(err)
	}

	if err := storage.UpdateServiceProfile(common.DB, &sp); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.UpdateServiceProfileResponse{}, nil
}

// DeleteServiceProfile deletes the service-profile matching the given id.
func (n *NetworkServerAPI) DeleteServiceProfile(ctx context.Context, req *ns.DeleteServiceProfileRequest) (*ns.DeleteServiceProfileResponse, error) {
	if err := storage.FlushServiceProfileCache(common.RedisPool, req.ServiceProfileID); err != nil {
		return nil, errToRPCError(err)
	}

	if err := storage.DeleteServiceProfile(common.DB, req.ServiceProfileID); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.DeleteServiceProfileResponse{}, nil
}

// CreateRoutingProfile creates the given routing-profile.
func (n *NetworkServerAPI) CreateRoutingProfile(ctx context.Context, req *ns.CreateRoutingProfileRequest) (*ns.CreateRoutingProfileResponse, error) {
	rp := storage.RoutingProfile{
		RoutingProfile: backend.RoutingProfile{
			RoutingProfileID: req.RoutingProfile.RoutingProfileID,
			ASID:             req.RoutingProfile.AsID,
		},
	}
	if err := storage.CreateRoutingProfile(common.DB, &rp); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.CreateRoutingProfileResponse{
		RoutingProfileID: rp.RoutingProfile.RoutingProfileID,
	}, nil
}

// GetRoutingProfile returns the routing-profile matching the given id.
func (n *NetworkServerAPI) GetRoutingProfile(ctx context.Context, req *ns.GetRoutingProfileRequest) (*ns.GetRoutingProfileResponse, error) {
	rp, err := storage.GetRoutingProfile(common.DB, req.RoutingProfileID)
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.GetRoutingProfileResponse{
		CreatedAt: rp.CreatedAt.Format(time.RFC3339Nano),
		UpdatedAt: rp.UpdatedAt.Format(time.RFC3339Nano),
		RoutingProfile: &ns.RoutingProfile{
			AsID: rp.RoutingProfile.ASID,
		},
	}, nil
}

// UpdateRoutingProfile updates the given routing-profile.
func (n *NetworkServerAPI) UpdateRoutingProfile(ctx context.Context, req *ns.UpdateRoutingProfileRequest) (*ns.UpdateRoutingProfileResponse, error) {
	rp, err := storage.GetRoutingProfile(common.DB, req.RoutingProfile.RoutingProfileID)
	if err != nil {
		return nil, errToRPCError(err)
	}

	rp.RoutingProfile = backend.RoutingProfile{
		RoutingProfileID: rp.RoutingProfile.RoutingProfileID,
		ASID:             req.RoutingProfile.AsID,
	}
	if err := storage.UpdateRoutingProfile(common.DB, &rp); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.UpdateRoutingProfileResponse{}, nil
}

// DeleteRoutingProfile deletes the routing-profile matching the given id.
func (n *NetworkServerAPI) DeleteRoutingProfile(ctx context.Context, req *ns.DeleteRoutingProfileRequest) (*ns.DeleteRoutingProfileResponse, error) {
	if err := storage.DeleteRoutingProfile(common.DB, req.RoutingProfileID); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.DeleteRoutingProfileResponse{}, nil
}

// CreateDeviceProfile creates the given device-profile.
// The RFRegion field will get set automatically according to the configured band.
func (n *NetworkServerAPI) CreateDeviceProfile(ctx context.Context, req *ns.CreateDeviceProfileRequest) (*ns.CreateDeviceProfileResponse, error) {
	var factoryPresetFreqs []backend.Frequency
	for _, f := range req.DeviceProfile.FactoryPresetFreqs {
		factoryPresetFreqs = append(factoryPresetFreqs, backend.Frequency(f))
	}

	dp := storage.DeviceProfile{
		DeviceProfile: backend.DeviceProfile{
			DeviceProfileID:    req.DeviceProfile.DeviceProfileID,
			SupportsClassB:     req.DeviceProfile.SupportsClassB,
			ClassBTimeout:      int(req.DeviceProfile.ClassBTimeout),
			PingSlotPeriod:     int(req.DeviceProfile.PingSlotPeriod),
			PingSlotDR:         int(req.DeviceProfile.PingSlotDR),
			PingSlotFreq:       backend.Frequency(req.DeviceProfile.PingSlotFreq),
			SupportsClassC:     req.DeviceProfile.SupportsClassC,
			ClassCTimeout:      int(req.DeviceProfile.ClassCTimeout),
			MACVersion:         req.DeviceProfile.MacVersion,
			RegParamsRevision:  req.DeviceProfile.RegParamsRevision,
			RXDelay1:           int(req.DeviceProfile.RxDelay1),
			RXDROffset1:        int(req.DeviceProfile.RxDROffset1),
			RXDataRate2:        int(req.DeviceProfile.RxDataRate2),
			RXFreq2:            backend.Frequency(req.DeviceProfile.RxFreq2),
			FactoryPresetFreqs: factoryPresetFreqs,
			MaxEIRP:            int(req.DeviceProfile.MaxEIRP),
			MaxDutyCycle:       backend.Percentage(req.DeviceProfile.MaxDutyCycle),
			SupportsJoin:       req.DeviceProfile.SupportsJoin,
			Supports32bitFCnt:  req.DeviceProfile.Supports32BitFCnt,
		},
	}

	var ok bool
	dp.DeviceProfile.RFRegion, ok = rfRegionMapping[common.BandName]
	if !ok {
		// band name has not been specified by the LoRaWAN backend interfaces
		// specification. use the internal BandName for now so that when these
		// values are specified in a next version, this can be fixed in a db
		// migration
		dp.DeviceProfile.RFRegion = backend.RFRegion(common.BandName)
	}

	if err := storage.CreateDeviceProfile(common.DB, &dp); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.CreateDeviceProfileResponse{
		DeviceProfileID: dp.DeviceProfile.DeviceProfileID,
	}, nil
}

// GetDeviceProfile returns the device-profile matching the given id.
func (n *NetworkServerAPI) GetDeviceProfile(ctx context.Context, req *ns.GetDeviceProfileRequest) (*ns.GetDeviceProfileResponse, error) {
	dp, err := storage.GetDeviceProfile(common.DB, req.DeviceProfileID)
	if err != nil {
		return nil, errToRPCError(err)
	}

	var factoryPresetFreqs []uint32
	for _, f := range dp.DeviceProfile.FactoryPresetFreqs {
		factoryPresetFreqs = append(factoryPresetFreqs, uint32(f))
	}

	resp := ns.GetDeviceProfileResponse{
		CreatedAt: dp.CreatedAt.Format(time.RFC3339Nano),
		UpdatedAt: dp.UpdatedAt.Format(time.RFC3339Nano),
		DeviceProfile: &ns.DeviceProfile{
			SupportsClassB:     dp.DeviceProfile.SupportsClassB,
			ClassBTimeout:      uint32(dp.DeviceProfile.ClassBTimeout),
			PingSlotPeriod:     uint32(dp.DeviceProfile.PingSlotPeriod),
			PingSlotDR:         uint32(dp.DeviceProfile.PingSlotDR),
			PingSlotFreq:       uint32(dp.DeviceProfile.PingSlotFreq),
			SupportsClassC:     dp.DeviceProfile.SupportsClassC,
			ClassCTimeout:      uint32(dp.DeviceProfile.ClassCTimeout),
			MacVersion:         dp.DeviceProfile.MACVersion,
			RegParamsRevision:  dp.DeviceProfile.RegParamsRevision,
			RxDelay1:           uint32(dp.DeviceProfile.RXDelay1),
			RxDROffset1:        uint32(dp.DeviceProfile.RXDROffset1),
			RxDataRate2:        uint32(dp.DeviceProfile.RXDataRate2),
			RxFreq2:            uint32(dp.DeviceProfile.RXFreq2),
			FactoryPresetFreqs: factoryPresetFreqs,
			MaxEIRP:            uint32(dp.DeviceProfile.MaxEIRP),
			MaxDutyCycle:       uint32(dp.DeviceProfile.MaxDutyCycle),
			SupportsJoin:       dp.DeviceProfile.SupportsJoin,
			RfRegion:           string(dp.DeviceProfile.RFRegion),
			Supports32BitFCnt:  dp.DeviceProfile.Supports32bitFCnt,
		},
	}

	return &resp, nil
}

// UpdateDeviceProfile updates the given device-profile.
// The RFRegion field will get set automatically according to the configured band.
func (n *NetworkServerAPI) UpdateDeviceProfile(ctx context.Context, req *ns.UpdateDeviceProfileRequest) (*ns.UpdateDeviceProfileResponse, error) {
	dp, err := storage.GetDeviceProfile(common.DB, req.DeviceProfile.DeviceProfileID)
	if err != nil {
		return nil, errToRPCError(err)
	}

	var factoryPresetFreqs []backend.Frequency
	for _, f := range req.DeviceProfile.FactoryPresetFreqs {
		factoryPresetFreqs = append(factoryPresetFreqs, backend.Frequency(f))
	}

	dp.DeviceProfile = backend.DeviceProfile{
		DeviceProfileID:    dp.DeviceProfile.DeviceProfileID,
		SupportsClassB:     req.DeviceProfile.SupportsClassB,
		PingSlotPeriod:     int(req.DeviceProfile.PingSlotPeriod),
		PingSlotDR:         int(req.DeviceProfile.PingSlotDR),
		PingSlotFreq:       backend.Frequency(req.DeviceProfile.PingSlotFreq),
		SupportsClassC:     req.DeviceProfile.SupportsClassC,
		ClassCTimeout:      int(req.DeviceProfile.ClassCTimeout),
		MACVersion:         req.DeviceProfile.MacVersion,
		RegParamsRevision:  req.DeviceProfile.RegParamsRevision,
		RXDelay1:           int(req.DeviceProfile.RxDelay1),
		RXDROffset1:        int(req.DeviceProfile.RxDROffset1),
		RXDataRate2:        int(req.DeviceProfile.RxDataRate2),
		RXFreq2:            backend.Frequency(req.DeviceProfile.RxFreq2),
		FactoryPresetFreqs: factoryPresetFreqs,
		MaxEIRP:            int(req.DeviceProfile.MaxEIRP),
		MaxDutyCycle:       backend.Percentage(req.DeviceProfile.MaxDutyCycle),
		SupportsJoin:       req.DeviceProfile.SupportsJoin,
		Supports32bitFCnt:  req.DeviceProfile.Supports32BitFCnt,
	}

	var ok bool
	dp.DeviceProfile.RFRegion, ok = rfRegionMapping[common.BandName]
	if !ok {
		// band name has not been specified by the LoRaWAN backend interfaces
		// specification. use the internal BandName for now so that when these
		// values are specified in a next version, this can be fixed in a db
		// migration
		dp.DeviceProfile.RFRegion = backend.RFRegion(common.BandName)
	}

	if err := storage.FlushDeviceProfileCache(common.RedisPool, dp.DeviceProfile.DeviceProfileID); err != nil {
		return nil, errToRPCError(err)
	}

	if err := storage.UpdateDeviceProfile(common.DB, &dp); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.UpdateDeviceProfileResponse{}, nil
}

// DeleteDeviceProfile deletes the device-profile matching the given id.
func (n *NetworkServerAPI) DeleteDeviceProfile(ctx context.Context, req *ns.DeleteDeviceProfileRequest) (*ns.DeleteDeviceProfileResponse, error) {
	if err := storage.FlushDeviceProfileCache(common.RedisPool, req.DeviceProfileID); err != nil {
		return nil, errToRPCError(err)
	}

	if err := storage.DeleteDeviceProfile(common.DB, req.DeviceProfileID); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.DeleteDeviceProfileResponse{}, nil
}

// CreateDevice creates the given device.
func (n *NetworkServerAPI) CreateDevice(ctx context.Context, req *ns.CreateDeviceRequest) (*ns.CreateDeviceResponse, error) {
	var devEUI lorawan.EUI64
	copy(devEUI[:], req.Device.DevEUI)

	d := storage.Device{
		DevEUI:           devEUI,
		DeviceProfileID:  req.Device.DeviceProfileID,
		ServiceProfileID: req.Device.ServiceProfileID,
		RoutingProfileID: req.Device.RoutingProfileID,
	}
	if err := storage.CreateDevice(common.DB, &d); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.CreateDeviceResponse{}, nil
}

// GetDevice returns the device matching the given DevEUI.
func (n *NetworkServerAPI) GetDevice(ctx context.Context, req *ns.GetDeviceRequest) (*ns.GetDeviceResponse, error) {
	var devEUI lorawan.EUI64
	copy(devEUI[:], req.DevEUI)

	d, err := storage.GetDevice(common.DB, devEUI)
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.GetDeviceResponse{
		CreatedAt: d.CreatedAt.Format(time.RFC3339Nano),
		UpdatedAt: d.UpdatedAt.Format(time.RFC3339Nano),
		Device: &ns.Device{
			DevEUI:           d.DevEUI[:],
			DeviceProfileID:  d.DeviceProfileID,
			ServiceProfileID: d.ServiceProfileID,
			RoutingProfileID: d.RoutingProfileID,
		},
	}, nil
}

// UpdateDevice updates the given device.
func (n *NetworkServerAPI) UpdateDevice(ctx context.Context, req *ns.UpdateDeviceRequest) (*ns.UpdateDeviceResponse, error) {
	var devEUI lorawan.EUI64
	copy(devEUI[:], req.Device.DevEUI)

	d, err := storage.GetDevice(common.DB, devEUI)
	if err != nil {
		return nil, errToRPCError(err)
	}

	d.DeviceProfileID = req.Device.DeviceProfileID
	d.ServiceProfileID = req.Device.ServiceProfileID
	d.RoutingProfileID = req.Device.RoutingProfileID

	if err := storage.UpdateDevice(common.DB, &d); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.UpdateDeviceResponse{}, nil
}

// DeleteDevice deletes the device matching the given DevEUI.
func (n *NetworkServerAPI) DeleteDevice(ctx context.Context, req *ns.DeleteDeviceRequest) (*ns.DeleteDeviceResponse, error) {
	var devEUI lorawan.EUI64
	copy(devEUI[:], req.DevEUI)

	if err := storage.DeleteDevice(common.DB, devEUI); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.DeleteDeviceResponse{}, nil
}

// ActivateDevice activates a device (ABP).
func (n *NetworkServerAPI) ActivateDevice(ctx context.Context, req *ns.ActivateDeviceRequest) (*ns.ActivateDeviceResponse, error) {
	var devEUI lorawan.EUI64
	var devAddr lorawan.DevAddr
	var nwkSKey lorawan.AES128Key

	copy(devEUI[:], req.DevEUI)
	copy(devAddr[:], req.DevAddr)
	copy(nwkSKey[:], req.NwkSKey)

	d, err := storage.GetDevice(common.DB, devEUI)
	if err != nil {
		return nil, errToRPCError(err)
	}

	sp, err := storage.GetServiceProfile(common.DB, d.ServiceProfileID)
	if err != nil {
		return nil, errToRPCError(err)
	}

	dp, err := storage.GetDeviceProfile(common.DB, d.DeviceProfileID)
	if err != nil {
		return nil, errToRPCError(err)
	}

	var channelFrequencies []int
	for _, f := range dp.FactoryPresetFreqs {
		channelFrequencies = append(channelFrequencies, int(f))
	}

	ds := storage.DeviceSession{
		DeviceProfileID:  d.DeviceProfileID,
		ServiceProfileID: d.ServiceProfileID,
		RoutingProfileID: d.RoutingProfileID,

		DevEUI:             devEUI,
		DevAddr:            devAddr,
		NwkSKey:            nwkSKey,
		FCntUp:             req.FCntUp,
		FCntDown:           req.FCntDown,
		SkipFCntValidation: req.SkipFCntCheck,

		RXWindow:       storage.RX1,
		RXDelay:        uint8(dp.RXDelay1),
		RX1DROffset:    uint8(dp.RXDROffset1),
		RX2DR:          uint8(dp.RXDataRate2),
		RX2Frequency:   int(dp.RXFreq2),
		MaxSupportedDR: sp.ServiceProfile.DRMax,

		EnabledChannels:    common.Band.GetUplinkChannels(), // TODO: replace by ServiceProfile.ChannelMask?
		ChannelFrequencies: channelFrequencies,
	}

	if err := storage.SaveDeviceSession(common.RedisPool, ds); err != nil {
		return nil, errToRPCError(err)
	}

	if err := storage.FlushDeviceQueueForDevEUI(common.DB, devEUI); err != nil {
		return nil, errToRPCError(err)
	}

	if err := maccommand.FlushQueue(common.RedisPool, ds.DevEUI); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.ActivateDeviceResponse{}, nil
}

// DeactivateDevice de-activates a device.
func (n *NetworkServerAPI) DeactivateDevice(ctx context.Context, req *ns.DeactivateDeviceRequest) (*ns.DeactivateDeviceResponse, error) {
	var devEUI lorawan.EUI64
	copy(devEUI[:], req.DevEUI)

	if err := storage.DeleteDeviceSession(common.RedisPool, devEUI); err != nil {
		return nil, errToRPCError(err)
	}

	if err := storage.FlushDeviceQueueForDevEUI(common.DB, devEUI); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.DeactivateDeviceResponse{}, nil
}

// GetDeviceActivation returns the device activation details.
func (n *NetworkServerAPI) GetDeviceActivation(ctx context.Context, req *ns.GetDeviceActivationRequest) (*ns.GetDeviceActivationResponse, error) {
	var devEUI lorawan.EUI64
	copy(devEUI[:], req.DevEUI)

	ds, err := storage.GetDeviceSession(common.RedisPool, devEUI)
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.GetDeviceActivationResponse{
		DevAddr:       ds.DevAddr[:],
		NwkSKey:       ds.NwkSKey[:],
		FCntUp:        ds.FCntUp,
		FCntDown:      ds.FCntDown,
		SkipFCntCheck: ds.SkipFCntValidation,
	}, nil
}

// GetRandomDevAddr returns a random DevAddr.
func (n *NetworkServerAPI) GetRandomDevAddr(ctx context.Context, req *ns.GetRandomDevAddrRequest) (*ns.GetRandomDevAddrResponse, error) {
	devAddr, err := storage.GetRandomDevAddr(common.RedisPool, common.NetID)
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.GetRandomDevAddrResponse{
		DevAddr: devAddr[:],
	}, nil
}

// EnqueueDownlinkMACCommand adds a data down MAC command to the queue.
// It replaces already enqueued mac-commands with the same CID.
func (n *NetworkServerAPI) EnqueueDownlinkMACCommand(ctx context.Context, req *ns.EnqueueDownlinkMACCommandRequest) (*ns.EnqueueDownlinkMACCommandResponse, error) {
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

	if err := maccommand.AddQueueItem(common.RedisPool, devEUI, block); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.EnqueueDownlinkMACCommandResponse{}, nil
}

// SendProprietaryPayload send a payload using the 'Proprietary' LoRaWAN message-type.
func (n *NetworkServerAPI) SendProprietaryPayload(ctx context.Context, req *ns.SendProprietaryPayloadRequest) (*ns.SendProprietaryPayloadResponse, error) {
	var mic lorawan.MIC
	var gwMACs []lorawan.EUI64

	copy(mic[:], req.Mic)
	for i := range req.GatewayMACs {
		var mac lorawan.EUI64
		copy(mac[:], req.GatewayMACs[i])
		gwMACs = append(gwMACs, mac)
	}

	err := downlink.Flow.RunProprietaryDown(req.MacPayload, mic, gwMACs, req.IPol, int(req.Frequency), int(req.Dr))
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.SendProprietaryPayloadResponse{}, nil
}

// CreateGateway creates the given gateway.
func (n *NetworkServerAPI) CreateGateway(ctx context.Context, req *ns.CreateGatewayRequest) (*ns.CreateGatewayResponse, error) {
	var mac lorawan.EUI64
	copy(mac[:], req.Mac)

	gw := gateway.Gateway{
		MAC:         mac,
		Name:        req.Name,
		Description: req.Description,
		Location: gateway.GPSPoint{
			Latitude:  req.Latitude,
			Longitude: req.Longitude,
		},
		Altitude: req.Altitude,
	}
	if req.ChannelConfigurationID != 0 {
		gw.ChannelConfigurationID = &req.ChannelConfigurationID
	}

	err := gateway.CreateGateway(common.DB, &gw)
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.CreateGatewayResponse{}, nil
}

// GetGateway returns data for a particular gateway.
func (n *NetworkServerAPI) GetGateway(ctx context.Context, req *ns.GetGatewayRequest) (*ns.GetGatewayResponse, error) {
	var mac lorawan.EUI64
	copy(mac[:], req.Mac)

	gw, err := gateway.GetGateway(common.DB, mac)
	if err != nil {
		return nil, errToRPCError(err)
	}

	return gwToResp(gw), nil
}

// UpdateGateway updates an existing gateway.
func (n *NetworkServerAPI) UpdateGateway(ctx context.Context, req *ns.UpdateGatewayRequest) (*ns.UpdateGatewayResponse, error) {
	var mac lorawan.EUI64
	copy(mac[:], req.Mac)

	gw, err := gateway.GetGateway(common.DB, mac)
	if err != nil {
		return nil, errToRPCError(err)
	}

	if req.ChannelConfigurationID != 0 {
		gw.ChannelConfigurationID = &req.ChannelConfigurationID
	} else {
		gw.ChannelConfigurationID = nil
	}

	gw.Name = req.Name
	gw.Description = req.Description
	gw.Location = gateway.GPSPoint{
		Latitude:  req.Latitude,
		Longitude: req.Longitude,
	}
	gw.Altitude = req.Altitude

	err = gateway.UpdateGateway(common.DB, &gw)
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.UpdateGatewayResponse{}, nil
}

// ListGateways returns the existing gateways.
func (n *NetworkServerAPI) ListGateways(ctx context.Context, req *ns.ListGatewayRequest) (*ns.ListGatewayResponse, error) {
	count, err := gateway.GetGatewayCount(common.DB)
	if err != nil {
		return nil, errToRPCError(err)
	}

	gws, err := gateway.GetGateways(common.DB, int(req.Limit), int(req.Offset))
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

	err := gateway.DeleteGateway(common.DB, mac)
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.DeleteGatewayResponse{}, nil
}

// GenerateGatewayToken issues a JWT token which can be used by the gateway
// for authentication.
func (n *NetworkServerAPI) GenerateGatewayToken(ctx context.Context, req *ns.GenerateGatewayTokenRequest) (*ns.GenerateGatewayTokenResponse, error) {
	var mac lorawan.EUI64
	copy(mac[:], req.Mac)

	// check that the gateway exists
	_, err := gateway.GetGateway(common.DB, mac)
	if err != nil {
		return nil, errToRPCError(err)
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, auth.Claims{
		StandardClaims: jwt.StandardClaims{
			Audience:  "ns",
			Issuer:    "ns",
			NotBefore: time.Now().Unix(),
			Subject:   "gateway",
		},
		MAC: mac,
	})
	signedToken, err := token.SignedString([]byte(common.GatewayServerJWTSecret))
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.GenerateGatewayTokenResponse{
		Token: signedToken,
	}, nil
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

	stats, err := gateway.GetGatewayStats(common.DB, mac, req.Interval.String(), start, end)
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

	count, err := node.GetFrameLogCountForDevEUI(common.DB, devEUI)
	if err != nil {
		return nil, errToRPCError(err)
	}

	logs, err := node.GetFrameLogsForDevEUI(common.DB, devEUI, int(req.Limit), int(req.Offset))
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

	if err := gateway.CreateChannelConfiguration(common.DB, &cf); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.CreateChannelConfigurationResponse{Id: cf.ID}, nil
}

// GetChannelConfiguration returns the channel-configuration for the given ID.
func (n *NetworkServerAPI) GetChannelConfiguration(ctx context.Context, req *ns.GetChannelConfigurationRequest) (*ns.GetChannelConfigurationResponse, error) {
	cf, err := gateway.GetChannelConfiguration(common.DB, req.Id)
	if err != nil {
		return nil, errToRPCError(err)
	}

	return channelConfigurationToResp(cf), nil
}

// UpdateChannelConfiguration updates the given channel-configuration.
func (n *NetworkServerAPI) UpdateChannelConfiguration(ctx context.Context, req *ns.UpdateChannelConfigurationRequest) (*ns.UpdateChannelConfigurationResponse, error) {
	cf, err := gateway.GetChannelConfiguration(common.DB, req.Id)
	if err != nil {
		return nil, errToRPCError(err)
	}

	cf.Name = req.Name
	cf.Channels = []int64{}
	for _, c := range req.Channels {
		cf.Channels = append(cf.Channels, int64(c))
	}

	if err = gateway.UpdateChannelConfiguration(common.DB, &cf); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.UpdateChannelConfigurationResponse{}, nil
}

// DeleteChannelConfiguration deletes the channel-configuration matching the
// given ID.
func (n *NetworkServerAPI) DeleteChannelConfiguration(ctx context.Context, req *ns.DeleteChannelConfigurationRequest) (*ns.DeleteChannelConfigurationResponse, error) {
	if err := gateway.DeleteChannelConfiguration(common.DB, req.Id); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.DeleteChannelConfigurationResponse{}, nil
}

// ListChannelConfigurations returns all channel-configurations.
func (n *NetworkServerAPI) ListChannelConfigurations(ctx context.Context, req *ns.ListChannelConfigurationsRequest) (*ns.ListChannelConfigurationsResponse, error) {
	cfs, err := gateway.GetChannelConfigurationsForBand(common.DB, string(common.BandName))
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
		BitRate:                int(req.BitRate),
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

	if err := gateway.CreateExtraChannel(common.DB, &ec); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.CreateExtraChannelResponse{Id: ec.ID}, nil
}

// UpdateExtraChannel updates the given extra channel.
func (n *NetworkServerAPI) UpdateExtraChannel(ctx context.Context, req *ns.UpdateExtraChannelRequest) (*ns.UpdateExtraChannelResponse, error) {
	ec, err := gateway.GetExtraChannel(common.DB, req.Id)
	if err != nil {
		return nil, errToRPCError(err)
	}

	ec.ChannelConfigurationID = req.ChannelConfigurationID
	ec.Frequency = int(req.Frequency)
	ec.BandWidth = int(req.BandWidth)
	ec.BitRate = int(req.BitRate)
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

	if err = gateway.UpdateExtraChannel(common.DB, &ec); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.UpdateExtraChannelResponse{}, nil
}

// DeleteExtraChannel deletes the extra channel matching the given id.
func (n *NetworkServerAPI) DeleteExtraChannel(ctx context.Context, req *ns.DeleteExtraChannelRequest) (*ns.DeleteExtraChannelResponse, error) {
	err := gateway.DeleteExtraChannel(common.DB, req.Id)
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.DeleteExtraChannelResponse{}, nil
}

// GetExtraChannelsForChannelConfigurationID returns the extra channels for
// the given channel-configuration id.
func (n *NetworkServerAPI) GetExtraChannelsForChannelConfigurationID(ctx context.Context, req *ns.GetExtraChannelsForChannelConfigurationIDRequest) (*ns.GetExtraChannelsForChannelConfigurationIDResponse, error) {
	chans, err := gateway.GetExtraChannelsForChannelConfigurationID(common.DB, req.Id)
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
			BitRate:                int32(c.BitRate),
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

// MigrateNodeToDeviceSession migrates a node-session to device-session.
// This method is for internal us only.
func (n *NetworkServerAPI) MigrateNodeToDeviceSession(ctx context.Context, req *ns.MigrateNodeToDeviceSessionRequest) (*ns.MigrateNodeToDeviceSessionResponse, error) {
	var devEUI lorawan.EUI64
	var joinEUI lorawan.EUI64
	var nonces []lorawan.DevNonce

	copy(devEUI[:], req.DevEUI)
	copy(joinEUI[:], req.JoinEUI)

	for _, nBytes := range req.DevNonces {
		var n lorawan.DevNonce
		copy(n[:], nBytes)
		nonces = append(nonces, n)
	}
	err := storage.Transaction(common.DB, func(tx *sqlx.Tx) error {
		if err := storage.MigrateNodeToDeviceSession(common.RedisPool, common.DB, devEUI, joinEUI, nonces); err != nil {
			return errToRPCError(err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return &ns.MigrateNodeToDeviceSessionResponse{}, nil
}

// CreateDeviceQueueItem creates the given device-queue item.
func (n *NetworkServerAPI) CreateDeviceQueueItem(ctx context.Context, req *ns.CreateDeviceQueueItemRequest) (*ns.CreateDeviceQueueItemResponse, error) {
	if req.Item == nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "item must not be nil")
	}

	var devEUI lorawan.EUI64
	copy(devEUI[:], req.Item.DevEUI)

	qi := storage.DeviceQueueItem{
		DevEUI:     devEUI,
		FRMPayload: req.Item.FrmPayload,
		FCnt:       req.Item.FCnt,
		FPort:      uint8(req.Item.FPort),
		Confirmed:  req.Item.Confirmed,
	}
	err := storage.CreateDeviceQueueItem(common.DB, &qi)
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.CreateDeviceQueueItemResponse{}, nil
}

// FlushDeviceQueueForDevEUI flushes the device-queue for the given DevEUI.
func (n *NetworkServerAPI) FlushDeviceQueueForDevEUI(ctx context.Context, req *ns.FlushDeviceQueueForDevEUIRequest) (*ns.FlushDeviceQueueForDevEUIResponse, error) {
	var devEUI lorawan.EUI64
	copy(devEUI[:], req.DevEUI)

	err := storage.FlushDeviceQueueForDevEUI(common.DB, devEUI)
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.FlushDeviceQueueForDevEUIResponse{}, nil
}

// GetDeviceQueueItemsForDevEUI returns all device-queue items for the given DevEUI.
func (n *NetworkServerAPI) GetDeviceQueueItemsForDevEUI(ctx context.Context, req *ns.GetDeviceQueueItemsForDevEUIRequest) (*ns.GetDeviceQueueItemsForDevEUIResponse, error) {
	var devEUI lorawan.EUI64
	copy(devEUI[:], req.DevEUI)

	items, err := storage.GetDeviceQueueItemsForDevEUI(common.DB, devEUI)
	if err != nil {
		return nil, errToRPCError(err)
	}

	var out ns.GetDeviceQueueItemsForDevEUIResponse
	for i := range items {
		qi := ns.DeviceQueueItem{
			DevEUI:     items[i].DevEUI[:],
			FrmPayload: items[i].FRMPayload,
			FCnt:       items[i].FCnt,
			FPort:      uint32(items[i].FPort),
			Confirmed:  items[i].Confirmed,
		}

		out.Items = append(out.Items, &qi)
	}

	return &out, nil
}

// GetNextDownlinkFCntForDevEUI returns the next FCnt that must be used.
// This also takes device-queue items for the given DevEUI into consideration.
// In case the device is not activated, this will return an error as no
// device-session exists.
func (n *NetworkServerAPI) GetNextDownlinkFCntForDevEUI(ctx context.Context, req *ns.GetNextDownlinkFCntForDevEUIRequest) (*ns.GetNextDownlinkFCntForDevEUIResponse, error) {
	var devEUI lorawan.EUI64
	copy(devEUI[:], req.DevEUI)

	var resp ns.GetNextDownlinkFCntForDevEUIResponse

	ds, err := storage.GetDeviceSession(common.RedisPool, devEUI)
	if err != nil {
		return nil, errToRPCError(err)
	}
	resp.FCnt = ds.FCntDown

	items, err := storage.GetDeviceQueueItemsForDevEUI(common.DB, devEUI)
	if err != nil {
		return nil, errToRPCError(err)
	}
	if count := len(items); count != 0 {
		resp.FCnt = items[count-1].FCnt + 1 // we want the next usable frame-counter
	}

	return &resp, nil
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
		Latitude:    gw.Location.Latitude,
		Longitude:   gw.Location.Longitude,
		Altitude:    gw.Altitude,
	}

	if gw.FirstSeenAt != nil {
		resp.FirstSeenAt = gw.FirstSeenAt.Format(time.RFC3339Nano)
	}

	if gw.LastSeenAt != nil {
		resp.LastSeenAt = gw.LastSeenAt.Format(time.RFC3339Nano)
	}

	if gw.ChannelConfigurationID != nil {
		resp.ChannelConfigurationID = *gw.ChannelConfigurationID
	}

	return &resp
}
