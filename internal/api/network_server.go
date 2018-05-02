package api

import (
	"time"

	"github.com/brocaar/loraserver/internal/gps"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/brocaar/loraserver/api/ns"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/downlink/data/classb"
	proprietarydown "github.com/brocaar/loraserver/internal/downlink/proprietary"
	"github.com/brocaar/loraserver/internal/framelog"
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

// classBScheduleMargin contains a Class-B scheduling margin to make sure
// there is enough time between scheduling and the actual Class-B ping-slot.
const classBScheduleMargin = 5 * time.Second

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

	if err := storage.CreateServiceProfile(config.C.PostgreSQL.DB, &sp); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.CreateServiceProfileResponse{
		ServiceProfileID: sp.ServiceProfile.ServiceProfileID,
	}, nil
}

// GetServiceProfile returns the service-profile matching the given id.
func (n *NetworkServerAPI) GetServiceProfile(ctx context.Context, req *ns.GetServiceProfileRequest) (*ns.GetServiceProfileResponse, error) {
	sp, err := storage.GetServiceProfile(config.C.PostgreSQL.DB, req.ServiceProfileID)
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
	sp, err := storage.GetServiceProfile(config.C.PostgreSQL.DB, req.ServiceProfile.ServiceProfileID)
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

	if err := storage.FlushServiceProfileCache(config.C.Redis.Pool, sp.ServiceProfile.ServiceProfileID); err != nil {
		return nil, errToRPCError(err)
	}

	if err := storage.UpdateServiceProfile(config.C.PostgreSQL.DB, &sp); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.UpdateServiceProfileResponse{}, nil
}

// DeleteServiceProfile deletes the service-profile matching the given id.
func (n *NetworkServerAPI) DeleteServiceProfile(ctx context.Context, req *ns.DeleteServiceProfileRequest) (*ns.DeleteServiceProfileResponse, error) {
	if err := storage.FlushServiceProfileCache(config.C.Redis.Pool, req.ServiceProfileID); err != nil {
		return nil, errToRPCError(err)
	}

	if err := storage.DeleteServiceProfile(config.C.PostgreSQL.DB, req.ServiceProfileID); err != nil {
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
		CACert:  req.CaCert,
		TLSCert: req.TlsCert,
		TLSKey:  req.TlsKey,
	}
	if err := storage.CreateRoutingProfile(config.C.PostgreSQL.DB, &rp); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.CreateRoutingProfileResponse{
		RoutingProfileID: rp.RoutingProfile.RoutingProfileID,
	}, nil
}

// GetRoutingProfile returns the routing-profile matching the given id.
func (n *NetworkServerAPI) GetRoutingProfile(ctx context.Context, req *ns.GetRoutingProfileRequest) (*ns.GetRoutingProfileResponse, error) {
	rp, err := storage.GetRoutingProfile(config.C.PostgreSQL.DB, req.RoutingProfileID)
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.GetRoutingProfileResponse{
		CreatedAt: rp.CreatedAt.Format(time.RFC3339Nano),
		UpdatedAt: rp.UpdatedAt.Format(time.RFC3339Nano),
		RoutingProfile: &ns.RoutingProfile{
			AsID: rp.RoutingProfile.ASID,
		},
		CaCert:  rp.CACert,
		TlsCert: rp.TLSCert,
	}, nil
}

// UpdateRoutingProfile updates the given routing-profile.
func (n *NetworkServerAPI) UpdateRoutingProfile(ctx context.Context, req *ns.UpdateRoutingProfileRequest) (*ns.UpdateRoutingProfileResponse, error) {
	rp, err := storage.GetRoutingProfile(config.C.PostgreSQL.DB, req.RoutingProfile.RoutingProfileID)
	if err != nil {
		return nil, errToRPCError(err)
	}

	rp.RoutingProfile = backend.RoutingProfile{
		RoutingProfileID: rp.RoutingProfile.RoutingProfileID,
		ASID:             req.RoutingProfile.AsID,
	}
	rp.CACert = req.CaCert
	rp.TLSCert = req.TlsCert

	if req.TlsKey != "" {
		rp.TLSKey = req.TlsKey
	}

	if rp.TLSCert == "" {
		rp.TLSKey = ""
	}

	if err := storage.UpdateRoutingProfile(config.C.PostgreSQL.DB, &rp); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.UpdateRoutingProfileResponse{}, nil
}

// DeleteRoutingProfile deletes the routing-profile matching the given id.
func (n *NetworkServerAPI) DeleteRoutingProfile(ctx context.Context, req *ns.DeleteRoutingProfileRequest) (*ns.DeleteRoutingProfileResponse, error) {
	if err := storage.DeleteRoutingProfile(config.C.PostgreSQL.DB, req.RoutingProfileID); err != nil {
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
	dp.DeviceProfile.RFRegion, ok = rfRegionMapping[config.C.NetworkServer.Band.Name]
	if !ok {
		// band name has not been specified by the LoRaWAN backend interfaces
		// specification. use the internal BandName for now so that when these
		// values are specified in a next version, this can be fixed in a db
		// migration
		dp.DeviceProfile.RFRegion = backend.RFRegion(config.C.NetworkServer.Band.Name)
	}

	if err := storage.CreateDeviceProfile(config.C.PostgreSQL.DB, &dp); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.CreateDeviceProfileResponse{
		DeviceProfileID: dp.DeviceProfile.DeviceProfileID,
	}, nil
}

// GetDeviceProfile returns the device-profile matching the given id.
func (n *NetworkServerAPI) GetDeviceProfile(ctx context.Context, req *ns.GetDeviceProfileRequest) (*ns.GetDeviceProfileResponse, error) {
	dp, err := storage.GetDeviceProfile(config.C.PostgreSQL.DB, req.DeviceProfileID)
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
	dp, err := storage.GetDeviceProfile(config.C.PostgreSQL.DB, req.DeviceProfile.DeviceProfileID)
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
	}

	var ok bool
	dp.DeviceProfile.RFRegion, ok = rfRegionMapping[config.C.NetworkServer.Band.Name]
	if !ok {
		// band name has not been specified by the LoRaWAN backend interfaces
		// specification. use the internal BandName for now so that when these
		// values are specified in a next version, this can be fixed in a db
		// migration
		dp.DeviceProfile.RFRegion = backend.RFRegion(config.C.NetworkServer.Band.Name)
	}

	if err := storage.FlushDeviceProfileCache(config.C.Redis.Pool, dp.DeviceProfile.DeviceProfileID); err != nil {
		return nil, errToRPCError(err)
	}

	if err := storage.UpdateDeviceProfile(config.C.PostgreSQL.DB, &dp); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.UpdateDeviceProfileResponse{}, nil
}

// DeleteDeviceProfile deletes the device-profile matching the given id.
func (n *NetworkServerAPI) DeleteDeviceProfile(ctx context.Context, req *ns.DeleteDeviceProfileRequest) (*ns.DeleteDeviceProfileResponse, error) {
	if err := storage.FlushDeviceProfileCache(config.C.Redis.Pool, req.DeviceProfileID); err != nil {
		return nil, errToRPCError(err)
	}

	if err := storage.DeleteDeviceProfile(config.C.PostgreSQL.DB, req.DeviceProfileID); err != nil {
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
		SkipFCntCheck:    req.Device.SkipFCntCheck,
	}
	if err := storage.CreateDevice(config.C.PostgreSQL.DB, &d); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.CreateDeviceResponse{}, nil
}

// GetDevice returns the device matching the given DevEUI.
func (n *NetworkServerAPI) GetDevice(ctx context.Context, req *ns.GetDeviceRequest) (*ns.GetDeviceResponse, error) {
	var devEUI lorawan.EUI64
	copy(devEUI[:], req.DevEUI)

	d, err := storage.GetDevice(config.C.PostgreSQL.DB, devEUI)
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
			SkipFCntCheck:    d.SkipFCntCheck,
		},
	}, nil
}

// UpdateDevice updates the given device.
func (n *NetworkServerAPI) UpdateDevice(ctx context.Context, req *ns.UpdateDeviceRequest) (*ns.UpdateDeviceResponse, error) {
	var devEUI lorawan.EUI64
	copy(devEUI[:], req.Device.DevEUI)

	d, err := storage.GetDevice(config.C.PostgreSQL.DB, devEUI)
	if err != nil {
		return nil, errToRPCError(err)
	}

	d.DeviceProfileID = req.Device.DeviceProfileID
	d.ServiceProfileID = req.Device.ServiceProfileID
	d.RoutingProfileID = req.Device.RoutingProfileID
	d.SkipFCntCheck = req.Device.SkipFCntCheck

	if err := storage.UpdateDevice(config.C.PostgreSQL.DB, &d); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.UpdateDeviceResponse{}, nil
}

// DeleteDevice deletes the device matching the given DevEUI.
func (n *NetworkServerAPI) DeleteDevice(ctx context.Context, req *ns.DeleteDeviceRequest) (*ns.DeleteDeviceResponse, error) {
	var devEUI lorawan.EUI64
	copy(devEUI[:], req.DevEUI)

	err := storage.Transaction(config.C.PostgreSQL.DB, func(tx sqlx.Ext) error {
		if err := storage.DeleteDevice(tx, devEUI); err != nil {
			return errToRPCError(err)
		}

		if err := storage.DeleteDeviceSession(config.C.Redis.Pool, devEUI); err != nil && err != storage.ErrDoesNotExist {
			return errToRPCError(err)
		}

		return nil
	})
	if err != nil {
		return nil, err
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

	d, err := storage.GetDevice(config.C.PostgreSQL.DB, devEUI)
	if err != nil {
		return nil, errToRPCError(err)
	}

	sp, err := storage.GetServiceProfile(config.C.PostgreSQL.DB, d.ServiceProfileID)
	if err != nil {
		return nil, errToRPCError(err)
	}

	dp, err := storage.GetDeviceProfile(config.C.PostgreSQL.DB, d.DeviceProfileID)
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
		SkipFCntValidation: req.SkipFCntCheck || d.SkipFCntCheck,

		RXWindow:       storage.RX1,
		RXDelay:        uint8(dp.RXDelay1),
		RX1DROffset:    uint8(dp.RXDROffset1),
		RX2DR:          uint8(dp.RXDataRate2),
		RX2Frequency:   int(dp.RXFreq2),
		MaxSupportedDR: sp.ServiceProfile.DRMax,

		EnabledUplinkChannels: config.C.NetworkServer.Band.Band.GetStandardUplinkChannelIndices(), // TODO: replace by ServiceProfile.ChannelMask?
		ChannelFrequencies:    channelFrequencies,

		// set to invalid value to indicate we haven't received a status yet
		LastDevStatusMargin: 127,
		PingSlotDR:          dp.PingSlotDR,
		PingSlotFrequency:   int(dp.PingSlotFreq),
		NbTrans:             1,
	}

	if dp.PingSlotPeriod != 0 {
		ds.PingSlotNb = (1 << 12) / dp.PingSlotPeriod
	}

	if err := storage.SaveDeviceSession(config.C.Redis.Pool, ds); err != nil {
		return nil, errToRPCError(err)
	}

	if err := storage.FlushDeviceQueueForDevEUI(config.C.PostgreSQL.DB, devEUI); err != nil {
		return nil, errToRPCError(err)
	}

	if err := storage.FlushMACCommandQueue(config.C.Redis.Pool, ds.DevEUI); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.ActivateDeviceResponse{}, nil
}

// DeactivateDevice de-activates a device.
func (n *NetworkServerAPI) DeactivateDevice(ctx context.Context, req *ns.DeactivateDeviceRequest) (*ns.DeactivateDeviceResponse, error) {
	var devEUI lorawan.EUI64
	copy(devEUI[:], req.DevEUI)

	if err := storage.DeleteDeviceSession(config.C.Redis.Pool, devEUI); err != nil {
		return nil, errToRPCError(err)
	}

	if err := storage.FlushDeviceQueueForDevEUI(config.C.PostgreSQL.DB, devEUI); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.DeactivateDeviceResponse{}, nil
}

// GetDeviceActivation returns the device activation details.
func (n *NetworkServerAPI) GetDeviceActivation(ctx context.Context, req *ns.GetDeviceActivationRequest) (*ns.GetDeviceActivationResponse, error) {
	var devEUI lorawan.EUI64
	copy(devEUI[:], req.DevEUI)

	ds, err := storage.GetDeviceSession(config.C.Redis.Pool, devEUI)
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
	devAddr, err := storage.GetRandomDevAddr(config.C.Redis.Pool, config.C.NetworkServer.NetID)
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.GetRandomDevAddrResponse{
		DevAddr: devAddr[:],
	}, nil
}

// CreateMACCommandQueueItem adds a data down MAC command to the queue.
// It replaces already enqueued mac-commands with the same CID.
func (n *NetworkServerAPI) CreateMACCommandQueueItem(ctx context.Context, req *ns.CreateMACCommandQueueItemRequest) (*ns.CreateMACCommandQueueItemResponse, error) {
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

	block := storage.MACCommandBlock{
		CID:         lorawan.CID(req.Cid),
		External:    true,
		MACCommands: commands,
	}

	if err := storage.CreateMACCommandQueueItem(config.C.Redis.Pool, devEUI, block); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.CreateMACCommandQueueItemResponse{}, nil
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

	err := proprietarydown.Handle(req.MacPayload, mic, gwMACs, req.IPol, int(req.Frequency), int(req.Dr))
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.SendProprietaryPayloadResponse{}, nil
}

// CreateGateway creates the given gateway.
func (n *NetworkServerAPI) CreateGateway(ctx context.Context, req *ns.CreateGatewayRequest) (*ns.CreateGatewayResponse, error) {
	var mac lorawan.EUI64
	copy(mac[:], req.Mac)

	gw := storage.Gateway{
		MAC:         mac,
		Name:        req.Name,
		Description: req.Description,
		Location: storage.GPSPoint{
			Latitude:  req.Latitude,
			Longitude: req.Longitude,
		},
		Altitude: req.Altitude,
	}
	if req.GatewayProfileID != "" {
		gw.GatewayProfileID = &req.GatewayProfileID
	}

	err := storage.CreateGateway(config.C.PostgreSQL.DB, &gw)
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.CreateGatewayResponse{}, nil
}

// GetGateway returns data for a particular gateway.
func (n *NetworkServerAPI) GetGateway(ctx context.Context, req *ns.GetGatewayRequest) (*ns.GetGatewayResponse, error) {
	var mac lorawan.EUI64
	copy(mac[:], req.Mac)

	gw, err := storage.GetGateway(config.C.PostgreSQL.DB, mac)
	if err != nil {
		return nil, errToRPCError(err)
	}

	return gwToResp(gw), nil
}

// UpdateGateway updates an existing gateway.
func (n *NetworkServerAPI) UpdateGateway(ctx context.Context, req *ns.UpdateGatewayRequest) (*ns.UpdateGatewayResponse, error) {
	var mac lorawan.EUI64
	copy(mac[:], req.Mac)

	gw, err := storage.GetGateway(config.C.PostgreSQL.DB, mac)
	if err != nil {
		return nil, errToRPCError(err)
	}

	if req.GatewayProfileID != "" {
		gw.GatewayProfileID = &req.GatewayProfileID
	} else {
		gw.GatewayProfileID = nil
	}

	gw.Name = req.Name
	gw.Description = req.Description
	gw.Location = storage.GPSPoint{
		Latitude:  req.Latitude,
		Longitude: req.Longitude,
	}
	gw.Altitude = req.Altitude

	err = storage.UpdateGateway(config.C.PostgreSQL.DB, &gw)
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.UpdateGatewayResponse{}, nil
}

// DeleteGateway deletes a gateway.
func (n *NetworkServerAPI) DeleteGateway(ctx context.Context, req *ns.DeleteGatewayRequest) (*ns.DeleteGatewayResponse, error) {
	var mac lorawan.EUI64
	copy(mac[:], req.Mac)

	err := storage.DeleteGateway(config.C.PostgreSQL.DB, mac)
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

	stats, err := storage.GetGatewayStats(config.C.PostgreSQL.DB, mac, req.Interval.String(), start, end)
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

// StreamFrameLogsForGateway returns a stream of frames seen by the given gateway.
func (n *NetworkServerAPI) StreamFrameLogsForGateway(req *ns.StreamFrameLogsForGatewayRequest, srv ns.NetworkServer_StreamFrameLogsForGatewayServer) error {
	var mac lorawan.EUI64
	copy(mac[:], req.Mac)

	frameLogChan := make(chan framelog.FrameLog)

	go func() {
		err := framelog.GetFrameLogForGateway(srv.Context(), mac, frameLogChan)
		if err != nil {
			log.WithError(err).Error("get frame-log for gateway error")
		}
		close(frameLogChan)
	}()

	for fl := range frameLogChan {
		up, down, err := frameLogToUplinkAndDownlinkFrameLog(fl)
		if err != nil {
			log.WithError(err).Error("frame-log to uplink and downlink frame-log error")
			continue
		}

		var resp ns.StreamFrameLogsForGatewayResponse
		if up != nil {
			resp.UplinkFrames = append(resp.UplinkFrames, up)
		}

		if down != nil {
			resp.DownlinkFrames = append(resp.DownlinkFrames, down)
		}

		if err := srv.Send(&resp); err != nil {
			log.WithError(err).Error("error sending frame-log response")
		}
	}

	return nil
}

// StreamFrameLogsForDevice returns a stream of frames seen by the given device.
func (n *NetworkServerAPI) StreamFrameLogsForDevice(req *ns.StreamFrameLogsForDeviceRequest, srv ns.NetworkServer_StreamFrameLogsForDeviceServer) error {
	var devEUI lorawan.EUI64
	copy(devEUI[:], req.DevEUI)

	frameLogChan := make(chan framelog.FrameLog)

	go func() {
		err := framelog.GetFrameLogForDevice(srv.Context(), devEUI, frameLogChan)
		if err != nil {
			log.WithError(err).Error("get frame-log for device error")
		}
		close(frameLogChan)
	}()

	for fl := range frameLogChan {
		up, down, err := frameLogToUplinkAndDownlinkFrameLog(fl)
		if err != nil {
			log.WithError(err).Error("frame-log to uplink and downlink frame-log error")
			continue
		}

		var resp ns.StreamFrameLogsForDeviceResponse
		if up != nil {
			resp.UplinkFrames = append(resp.UplinkFrames, up)
		}

		if down != nil {
			resp.DownlinkFrames = append(resp.DownlinkFrames, down)
		}

		if err := srv.Send(&resp); err != nil {
			log.WithError(err).Error("error sending frame-log response")
		}
	}

	return nil
}

// CreateGatewayProfile creates the given gateway-profile.
func (n *NetworkServerAPI) CreateGatewayProfile(ctx context.Context, req *ns.CreateGatewayProfileRequest) (*ns.CreateGatewayProfileResponse, error) {
	gc := storage.GatewayProfile{
		GatewayProfileID: req.GatewayProfile.GatewayProfileID,
	}

	for _, c := range req.GatewayProfile.Channels {
		gc.Channels = append(gc.Channels, int64(c))
	}

	for _, ec := range req.GatewayProfile.ExtraChannels {
		c := storage.ExtraChannel{
			Frequency: int(ec.Frequency),
			Bandwidth: int(ec.Bandwidth),
			Bitrate:   int(ec.Bitrate),
		}

		switch ec.Modulation {
		case ns.Modulation_FSK:
			c.Modulation = storage.ModulationFSK
		default:
			c.Modulation = storage.ModulationLoRa
		}

		for _, sf := range ec.SpreadingFactors {
			c.SpreadingFactors = append(c.SpreadingFactors, int64(sf))
		}

		gc.ExtraChannels = append(gc.ExtraChannels, c)
	}

	err := storage.Transaction(config.C.PostgreSQL.DB, func(tx sqlx.Ext) error {
		return storage.CreateGatewayProfile(tx, &gc)
	})
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.CreateGatewayProfileResponse{GatewayProfileID: gc.GatewayProfileID}, nil
}

// GetGatewayProfile returns the gateway-profile given an id.
func (n *NetworkServerAPI) GetGatewayProfile(ctx context.Context, req *ns.GetGatewayProfileRequest) (*ns.GetGatewayProfileResponse, error) {
	gc, err := storage.GetGatewayProfile(config.C.PostgreSQL.DB, req.GatewayProfileID)
	if err != nil {
		return nil, errToRPCError(err)
	}

	out := ns.GetGatewayProfileResponse{
		CreatedAt: gc.CreatedAt.Format(time.RFC3339Nano),
		UpdatedAt: gc.UpdatedAt.Format(time.RFC3339Nano),
		GatewayProfile: &ns.GatewayProfile{
			GatewayProfileID: gc.GatewayProfileID,
		},
	}

	for _, c := range gc.Channels {
		out.GatewayProfile.Channels = append(out.GatewayProfile.Channels, uint32(c))
	}

	for _, ec := range gc.ExtraChannels {
		c := ns.GatewayProfileExtraChannel{
			Frequency: uint32(ec.Frequency),
			Bandwidth: uint32(ec.Bandwidth),
			Bitrate:   uint32(ec.Bitrate),
		}

		switch ec.Modulation {
		case storage.ModulationFSK:
			c.Modulation = ns.Modulation_FSK
		default:
			c.Modulation = ns.Modulation_LORA
		}

		for _, sf := range ec.SpreadingFactors {
			c.SpreadingFactors = append(c.SpreadingFactors, uint32(sf))
		}

		out.GatewayProfile.ExtraChannels = append(out.GatewayProfile.ExtraChannels, &c)
	}

	return &out, nil
}

// UpdateGatewayProfile updates the given gateway-profile.
func (n *NetworkServerAPI) UpdateGatewayProfile(ctx context.Context, req *ns.UpdateGatewayProfileRequest) (*ns.UpdateGatewayProfileResponse, error) {
	gc, err := storage.GetGatewayProfile(config.C.PostgreSQL.DB, req.GatewayProfile.GatewayProfileID)
	if err != nil {
		return nil, errToRPCError(err)
	}

	gc.Channels = []int64{}
	for _, c := range req.GatewayProfile.Channels {
		gc.Channels = append(gc.Channels, int64(c))
	}

	gc.ExtraChannels = []storage.ExtraChannel{}
	for _, ec := range req.GatewayProfile.ExtraChannels {
		c := storage.ExtraChannel{
			Frequency: int(ec.Frequency),
			Bandwidth: int(ec.Bandwidth),
			Bitrate:   int(ec.Bitrate),
		}

		switch ec.Modulation {
		case ns.Modulation_FSK:
			c.Modulation = storage.ModulationFSK
		default:
			c.Modulation = storage.ModulationLoRa
		}

		for _, sf := range ec.SpreadingFactors {
			c.SpreadingFactors = append(c.SpreadingFactors, int64(sf))
		}

		gc.ExtraChannels = append(gc.ExtraChannels, c)
	}

	err = storage.Transaction(config.C.PostgreSQL.DB, func(tx sqlx.Ext) error {
		return storage.UpdateGatewayProfile(tx, &gc)
	})
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.UpdateGatewayProfileResponse{}, nil
}

// DeleteGatewayProfile deletes the gateway-profile matching a given id.
func (n *NetworkServerAPI) DeleteGatewayProfile(ctx context.Context, req *ns.DeleteGatewayProfileRequest) (*ns.DeleteGatewayProfileResponse, error) {
	if err := storage.DeleteGatewayProfile(config.C.PostgreSQL.DB, req.GatewayProfileID); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.DeleteGatewayProfileResponse{}, nil
}

// CreateDeviceQueueItem creates the given device-queue item.
func (n *NetworkServerAPI) CreateDeviceQueueItem(ctx context.Context, req *ns.CreateDeviceQueueItemRequest) (*ns.CreateDeviceQueueItemResponse, error) {
	if req.Item == nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "item must not be nil")
	}

	var devEUI lorawan.EUI64
	copy(devEUI[:], req.Item.DevEUI)

	d, err := storage.GetDevice(config.C.PostgreSQL.DB, devEUI)
	if err != nil {
		return nil, errToRPCError(err)
	}

	dp, err := storage.GetDeviceProfile(config.C.PostgreSQL.DB, d.DeviceProfileID)
	if err != nil {
		return nil, errToRPCError(err)
	}

	qi := storage.DeviceQueueItem{
		DevEUI:     devEUI,
		FRMPayload: req.Item.FrmPayload,
		FCnt:       req.Item.FCnt,
		FPort:      uint8(req.Item.FPort),
		Confirmed:  req.Item.Confirmed,
	}

	// When the device is operating in Class-B and has a beacon lock, calculate
	// the next ping-slot.
	if dp.SupportsClassB {
		// check if device is currently active and is operating in Class-B mode
		ds, err := storage.GetDeviceSession(config.C.Redis.Pool, devEUI)
		if err != nil && err != storage.ErrDoesNotExist {
			return nil, errToRPCError(err)
		}

		if err == nil && ds.BeaconLocked {
			scheduleAfterGPSEpochTS, err := storage.GetMaxEmitAtTimeSinceGPSEpochForDevEUI(config.C.PostgreSQL.DB, devEUI)
			if err != nil {
				return nil, errToRPCError(err)
			}

			if scheduleAfterGPSEpochTS == 0 {
				scheduleAfterGPSEpochTS = gps.Time(time.Now()).TimeSinceGPSEpoch()
			}

			// take some margin into account
			scheduleAfterGPSEpochTS += classBScheduleMargin

			gpsEpochTS, err := classb.GetNextPingSlotAfter(scheduleAfterGPSEpochTS, ds.DevAddr, ds.PingSlotNb)
			if err != nil {
				return nil, errToRPCError(err)
			}

			timeoutTime := time.Time(gps.NewFromTimeSinceGPSEpoch(gpsEpochTS)).Add(time.Second * time.Duration(dp.ClassBTimeout))
			qi.EmitAtTimeSinceGPSEpoch = &gpsEpochTS
			qi.TimeoutAfter = &timeoutTime
		}
	}

	err = storage.CreateDeviceQueueItem(config.C.PostgreSQL.DB, &qi)
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.CreateDeviceQueueItemResponse{}, nil
}

// FlushDeviceQueueForDevEUI flushes the device-queue for the given DevEUI.
func (n *NetworkServerAPI) FlushDeviceQueueForDevEUI(ctx context.Context, req *ns.FlushDeviceQueueForDevEUIRequest) (*ns.FlushDeviceQueueForDevEUIResponse, error) {
	var devEUI lorawan.EUI64
	copy(devEUI[:], req.DevEUI)

	err := storage.FlushDeviceQueueForDevEUI(config.C.PostgreSQL.DB, devEUI)
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.FlushDeviceQueueForDevEUIResponse{}, nil
}

// GetDeviceQueueItemsForDevEUI returns all device-queue items for the given DevEUI.
func (n *NetworkServerAPI) GetDeviceQueueItemsForDevEUI(ctx context.Context, req *ns.GetDeviceQueueItemsForDevEUIRequest) (*ns.GetDeviceQueueItemsForDevEUIResponse, error) {
	var devEUI lorawan.EUI64
	copy(devEUI[:], req.DevEUI)

	items, err := storage.GetDeviceQueueItemsForDevEUI(config.C.PostgreSQL.DB, devEUI)
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

	ds, err := storage.GetDeviceSession(config.C.Redis.Pool, devEUI)
	if err != nil {
		return nil, errToRPCError(err)
	}
	resp.FCnt = ds.FCntDown

	items, err := storage.GetDeviceQueueItemsForDevEUI(config.C.PostgreSQL.DB, devEUI)
	if err != nil {
		return nil, errToRPCError(err)
	}
	if count := len(items); count != 0 {
		resp.FCnt = items[count-1].FCnt + 1 // we want the next usable frame-counter
	}

	return &resp, nil
}

// GetVersion returns the LoRa Server version.
func (n *NetworkServerAPI) GetVersion(ctx context.Context, req *ns.GetVersionRequest) (*ns.GetVersionResponse, error) {
	region, ok := map[band.Name]ns.Region{
		band.AS_923:     ns.Region_AS923,
		band.AU_915_928: ns.Region_AU915,
		band.CN_470_510: ns.Region_CN470,
		band.CN_779_787: ns.Region_CN779,
		band.EU_433:     ns.Region_EU433,
		band.EU_863_870: ns.Region_EU868,
		band.IN_865_867: ns.Region_IN865,
		band.KR_920_923: ns.Region_KR920,
		band.RU_864_870: ns.Region_RU864,
		band.US_902_928: ns.Region_US915,
	}[config.C.NetworkServer.Band.Name]

	if !ok {
		log.WithFields(log.Fields{
			"band_name": config.C.NetworkServer.Band.Name,
		}).Warning("unknown band to common name mapping")
	}

	return &ns.GetVersionResponse{
		Region:  region,
		Version: config.Version,
	}, nil
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
	err := storage.Transaction(config.C.PostgreSQL.DB, func(tx sqlx.Ext) error {
		if err := storage.MigrateNodeToDeviceSession(config.C.Redis.Pool, config.C.PostgreSQL.DB, devEUI, joinEUI, nonces); err != nil {
			return errToRPCError(err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return &ns.MigrateNodeToDeviceSessionResponse{}, nil
}

// MigrateChannelConfigurationToGatewayProfile migrates the channel configuration.
// This method is for internal use only.
func (n *NetworkServerAPI) MigrateChannelConfigurationToGatewayProfile(ctx context.Context, req *ns.MigrateChannelConfigurationToGatewayProfileRequest) (*ns.MigrateChannelConfigurationToGatewayProfileResponse, error) {
	var out ns.MigrateChannelConfigurationToGatewayProfileResponse

	err := storage.Transaction(config.C.PostgreSQL.DB, func(tx sqlx.Ext) error {
		migrated, err := storage.MigrateChannelConfigurationToGatewayProfile(tx)
		if err != nil {
			return err
		}

		for i := range migrated {
			gpm := ns.GatewayProfileMigration{
				GatewayProfileID: migrated[i].GatewayProfileID,
				Name:             migrated[i].Name,
			}

			for x := range migrated[i].Gateways {
				gpm.Macs = append(gpm.Macs, migrated[i].Gateways[x][:])
			}

			out.Migrated = append(out.Migrated, &gpm)
		}

		return nil
	})
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &out, nil
}

func gwToResp(gw storage.Gateway) *ns.GetGatewayResponse {
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

	if gw.GatewayProfileID != nil {
		resp.GatewayProfileID = *gw.GatewayProfileID
	}

	return &resp
}

func frameLogToUplinkAndDownlinkFrameLog(fl framelog.FrameLog) (*ns.UplinkFrameLog, *ns.DownlinkFrameLog, error) {
	var up *ns.UplinkFrameLog
	var down *ns.DownlinkFrameLog

	if fl.UplinkFrame != nil {
		b, err := fl.UplinkFrame.PHYPayload.MarshalBinary()
		if err != nil {
			return nil, nil, errors.Wrap(err, "marshal phypayload error")
		}

		up = &ns.UplinkFrameLog{
			TxInfo: &ns.UplinkTXInfo{
				Frequency: uint32(fl.UplinkFrame.TXInfo.Frequency),
				DataRate: &ns.DataRate{
					Modulation:   string(fl.UplinkFrame.TXInfo.DataRate.Modulation),
					Bandwidth:    uint32(fl.UplinkFrame.TXInfo.DataRate.Bandwidth),
					SpreadFactor: uint32(fl.UplinkFrame.TXInfo.DataRate.SpreadFactor),
					Bitrate:      uint32(fl.UplinkFrame.TXInfo.DataRate.BitRate),
				},
				CodeRate: fl.UplinkFrame.TXInfo.CodeRate,
			},
			PhyPayload: b,
		}

		for i := range fl.UplinkFrame.RXInfoSet {
			rxInfo := ns.UplinkRXInfo{
				Mac:       fl.UplinkFrame.RXInfoSet[i].MAC[:],
				Timestamp: fl.UplinkFrame.RXInfoSet[i].Timestamp,
				Rssi:      int32(fl.UplinkFrame.RXInfoSet[i].RSSI),
				LoRaSNR:   float32(fl.UplinkFrame.RXInfoSet[i].LoRaSNR),
				Board:     uint32(fl.UplinkFrame.RXInfoSet[i].Board),
				Antenna:   uint32(fl.UplinkFrame.RXInfoSet[i].Antenna),
			}

			if fl.UplinkFrame.RXInfoSet[i].Time != nil {
				rxInfo.Time = fl.UplinkFrame.RXInfoSet[i].Time.Format(time.RFC3339Nano)
			}

			if fl.UplinkFrame.RXInfoSet[i].TimeSinceGPSEpoch != nil {
				rxInfo.TimeSinceGPSEpoch = time.Duration(*fl.UplinkFrame.RXInfoSet[i].TimeSinceGPSEpoch).String()
			}

			up.RxInfo = append(up.RxInfo, &rxInfo)
		}
	}

	if fl.DownlinkFrame != nil {
		b, err := fl.DownlinkFrame.PHYPayload.MarshalBinary()
		if err != nil {
			return nil, nil, errors.Wrap(err, "marshal phypayload error")
		}

		down = &ns.DownlinkFrameLog{
			TxInfo: &ns.DownlinkTXInfo{
				Mac:         fl.DownlinkFrame.TXInfo.MAC[:],
				Immediately: fl.DownlinkFrame.TXInfo.Immediately,
				Frequency:   uint32(fl.DownlinkFrame.TXInfo.Frequency),
				Power:       int32(fl.DownlinkFrame.TXInfo.Power),
				DataRate: &ns.DataRate{
					Modulation:   string(fl.DownlinkFrame.TXInfo.DataRate.Modulation),
					Bandwidth:    uint32(fl.DownlinkFrame.TXInfo.DataRate.Bandwidth),
					SpreadFactor: uint32(fl.DownlinkFrame.TXInfo.DataRate.SpreadFactor),
					Bitrate:      uint32(fl.DownlinkFrame.TXInfo.DataRate.BitRate),
				},
				CodeRate: fl.DownlinkFrame.TXInfo.CodeRate,
				Board:    uint32(fl.DownlinkFrame.TXInfo.Board),
				Antenna:  uint32(fl.DownlinkFrame.TXInfo.Antenna),
			},
			PhyPayload: b,
		}

		if fl.DownlinkFrame.TXInfo.Timestamp != nil {
			down.TxInfo.Timestamp = uint32(*fl.DownlinkFrame.TXInfo.Timestamp)
		}

		if fl.DownlinkFrame.TXInfo.TimeSinceGPSEpoch != nil {
			down.TxInfo.TimeSinceGPSEpoch = time.Duration(*fl.DownlinkFrame.TXInfo.TimeSinceGPSEpoch).String()
		}

		if fl.DownlinkFrame.TXInfo.IPol != nil {
			down.TxInfo.IPol = *fl.DownlinkFrame.TXInfo.IPol
		} else {
			down.TxInfo.IPol = true
		}
	}

	return up, down, nil
}
