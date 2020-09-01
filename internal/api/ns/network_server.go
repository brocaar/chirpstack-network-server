package ns

import (
	"time"

	"github.com/gofrs/uuid"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/jmoiron/sqlx"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/brocaar/chirpstack-api/go/v3/common"
	"github.com/brocaar/chirpstack-api/go/v3/ns"
	"github.com/brocaar/chirpstack-network-server/internal/band"
	"github.com/brocaar/chirpstack-network-server/internal/config"
	"github.com/brocaar/chirpstack-network-server/internal/downlink/data/classb"
	"github.com/brocaar/chirpstack-network-server/internal/downlink/multicast"
	proprietarydown "github.com/brocaar/chirpstack-network-server/internal/downlink/proprietary"
	"github.com/brocaar/chirpstack-network-server/internal/framelog"
	"github.com/brocaar/chirpstack-network-server/internal/gateway"
	"github.com/brocaar/chirpstack-network-server/internal/gps"
	"github.com/brocaar/chirpstack-network-server/internal/helpers"
	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/backend"
)

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
	if req.ServiceProfile == nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "service_profile must not be nil")
	}

	var spID uuid.UUID
	copy(spID[:], req.ServiceProfile.Id)

	sp := storage.ServiceProfile{
		ID:                     spID,
		ULRate:                 int(req.ServiceProfile.UlRate),
		ULBucketSize:           int(req.ServiceProfile.UlBucketSize),
		DLRate:                 int(req.ServiceProfile.DlRate),
		DLBucketSize:           int(req.ServiceProfile.DlBucketSize),
		AddGWMetadata:          req.ServiceProfile.AddGwMetadata,
		DevStatusReqFreq:       int(req.ServiceProfile.DevStatusReqFreq),
		ReportDevStatusBattery: req.ServiceProfile.ReportDevStatusBattery,
		ReportDevStatusMargin:  req.ServiceProfile.ReportDevStatusMargin,
		DRMin:                  int(req.ServiceProfile.DrMin),
		DRMax:                  int(req.ServiceProfile.DrMax),
		ChannelMask:            req.ServiceProfile.ChannelMask,
		PRAllowed:              req.ServiceProfile.PrAllowed,
		HRAllowed:              req.ServiceProfile.HrAllowed,
		RAAllowed:              req.ServiceProfile.RaAllowed,
		NwkGeoLoc:              req.ServiceProfile.NwkGeoLoc,
		TargetPER:              int(req.ServiceProfile.TargetPer),
		MinGWDiversity:         int(req.ServiceProfile.MinGwDiversity),
	}

	switch req.ServiceProfile.UlRatePolicy {
	case ns.RatePolicy_MARK:
		sp.ULRatePolicy = storage.Mark
	case ns.RatePolicy_DROP:
		sp.ULRatePolicy = storage.Drop
	}

	switch req.ServiceProfile.DlRatePolicy {
	case ns.RatePolicy_MARK:
		sp.DLRatePolicy = storage.Mark
	case ns.RatePolicy_DROP:
		sp.DLRatePolicy = storage.Drop
	}

	if err := storage.CreateServiceProfile(ctx, storage.DB(), &sp); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.CreateServiceProfileResponse{
		Id: sp.ID.Bytes(),
	}, nil
}

// GetServiceProfile returns the service-profile matching the given id.
func (n *NetworkServerAPI) GetServiceProfile(ctx context.Context, req *ns.GetServiceProfileRequest) (*ns.GetServiceProfileResponse, error) {
	var spID uuid.UUID
	copy(spID[:], req.Id)

	sp, err := storage.GetServiceProfile(ctx, storage.DB(), spID)
	if err != nil {
		return nil, errToRPCError(err)
	}

	resp := ns.GetServiceProfileResponse{
		ServiceProfile: &ns.ServiceProfile{
			Id:                     sp.ID.Bytes(),
			UlRate:                 uint32(sp.ULRate),
			UlBucketSize:           uint32(sp.ULBucketSize),
			DlRate:                 uint32(sp.DLRate),
			DlBucketSize:           uint32(sp.DLBucketSize),
			AddGwMetadata:          sp.AddGWMetadata,
			DevStatusReqFreq:       uint32(sp.DevStatusReqFreq),
			ReportDevStatusBattery: sp.ReportDevStatusBattery,
			ReportDevStatusMargin:  sp.ReportDevStatusMargin,
			DrMin:                  uint32(sp.DRMin),
			DrMax:                  uint32(sp.DRMax),
			ChannelMask:            sp.ChannelMask,
			PrAllowed:              sp.PRAllowed,
			HrAllowed:              sp.HRAllowed,
			RaAllowed:              sp.RAAllowed,
			NwkGeoLoc:              sp.NwkGeoLoc,
			TargetPer:              uint32(sp.TargetPER),
			MinGwDiversity:         uint32(sp.MinGWDiversity),
		},
	}

	resp.CreatedAt, err = ptypes.TimestampProto(sp.CreatedAt)
	if err != nil {
		return nil, errToRPCError(err)
	}

	resp.UpdatedAt, err = ptypes.TimestampProto(sp.UpdatedAt)
	if err != nil {
		return nil, errToRPCError(err)
	}

	switch sp.ULRatePolicy {
	case storage.Mark:
		resp.ServiceProfile.UlRatePolicy = ns.RatePolicy_MARK
	case storage.Drop:
		resp.ServiceProfile.UlRatePolicy = ns.RatePolicy_DROP
	}

	switch sp.DLRatePolicy {
	case storage.Mark:
		resp.ServiceProfile.DlRatePolicy = ns.RatePolicy_MARK
	case storage.Drop:
		resp.ServiceProfile.DlRatePolicy = ns.RatePolicy_DROP
	}

	return &resp, nil
}

// UpdateServiceProfile updates the given service-profile.
func (n *NetworkServerAPI) UpdateServiceProfile(ctx context.Context, req *ns.UpdateServiceProfileRequest) (*empty.Empty, error) {
	if req.ServiceProfile == nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "service_profile must not be nil")
	}

	var spID uuid.UUID
	copy(spID[:], req.ServiceProfile.Id)

	sp, err := storage.GetServiceProfile(ctx, storage.DB(), spID)
	if err != nil {
		return nil, errToRPCError(err)
	}

	sp.ULRate = int(req.ServiceProfile.UlRate)
	sp.ULBucketSize = int(req.ServiceProfile.UlBucketSize)
	sp.DLRate = int(req.ServiceProfile.DlRate)
	sp.DLBucketSize = int(req.ServiceProfile.DlBucketSize)
	sp.AddGWMetadata = req.ServiceProfile.AddGwMetadata
	sp.DevStatusReqFreq = int(req.ServiceProfile.DevStatusReqFreq)
	sp.ReportDevStatusBattery = req.ServiceProfile.ReportDevStatusBattery
	sp.ReportDevStatusMargin = req.ServiceProfile.ReportDevStatusMargin
	sp.DRMin = int(req.ServiceProfile.DrMin)
	sp.DRMax = int(req.ServiceProfile.DrMax)
	sp.ChannelMask = backend.HEXBytes(req.ServiceProfile.ChannelMask)
	sp.PRAllowed = req.ServiceProfile.PrAllowed
	sp.HRAllowed = req.ServiceProfile.HrAllowed
	sp.RAAllowed = req.ServiceProfile.RaAllowed
	sp.NwkGeoLoc = req.ServiceProfile.NwkGeoLoc
	sp.TargetPER = int(req.ServiceProfile.TargetPer)
	sp.MinGWDiversity = int(req.ServiceProfile.MinGwDiversity)

	switch req.ServiceProfile.UlRatePolicy {
	case ns.RatePolicy_MARK:
		sp.ULRatePolicy = storage.Mark
	case ns.RatePolicy_DROP:
		sp.ULRatePolicy = storage.Drop
	}

	switch req.ServiceProfile.DlRatePolicy {
	case ns.RatePolicy_MARK:
		sp.DLRatePolicy = storage.Mark
	case ns.RatePolicy_DROP:
		sp.DLRatePolicy = storage.Drop
	}

	if err := storage.FlushServiceProfileCache(ctx, sp.ID); err != nil {
		return nil, errToRPCError(err)
	}

	if err := storage.UpdateServiceProfile(ctx, storage.DB(), &sp); err != nil {
		return nil, errToRPCError(err)
	}

	return &empty.Empty{}, nil
}

// DeleteServiceProfile deletes the service-profile matching the given id.
func (n *NetworkServerAPI) DeleteServiceProfile(ctx context.Context, req *ns.DeleteServiceProfileRequest) (*empty.Empty, error) {
	var spID uuid.UUID
	copy(spID[:], req.Id)

	if err := storage.FlushServiceProfileCache(ctx, spID); err != nil {
		return nil, errToRPCError(err)
	}

	if err := storage.DeleteServiceProfile(ctx, storage.DB(), spID); err != nil {
		return nil, errToRPCError(err)
	}

	return &empty.Empty{}, nil
}

// CreateRoutingProfile creates the given routing-profile.
func (n *NetworkServerAPI) CreateRoutingProfile(ctx context.Context, req *ns.CreateRoutingProfileRequest) (*ns.CreateRoutingProfileResponse, error) {
	if req.RoutingProfile == nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "routing_profile must not be nil")
	}

	var rpID uuid.UUID
	copy(rpID[:], req.RoutingProfile.Id)

	rp := storage.RoutingProfile{
		ID:      rpID,
		ASID:    req.RoutingProfile.AsId,
		CACert:  req.RoutingProfile.CaCert,
		TLSCert: req.RoutingProfile.TlsCert,
		TLSKey:  req.RoutingProfile.TlsKey,
	}
	if err := storage.CreateRoutingProfile(ctx, storage.DB(), &rp); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.CreateRoutingProfileResponse{
		Id: rp.ID.Bytes(),
	}, nil
}

// GetRoutingProfile returns the routing-profile matching the given id.
func (n *NetworkServerAPI) GetRoutingProfile(ctx context.Context, req *ns.GetRoutingProfileRequest) (*ns.GetRoutingProfileResponse, error) {
	var rpID uuid.UUID
	copy(rpID[:], req.Id)

	rp, err := storage.GetRoutingProfile(ctx, storage.DB(), rpID)
	if err != nil {
		return nil, errToRPCError(err)
	}

	resp := ns.GetRoutingProfileResponse{
		RoutingProfile: &ns.RoutingProfile{
			Id:      rp.ID.Bytes(),
			AsId:    rp.ASID,
			CaCert:  rp.CACert,
			TlsCert: rp.TLSCert,
		},
	}

	resp.CreatedAt, err = ptypes.TimestampProto(rp.CreatedAt)
	if err != nil {
		return nil, errToRPCError(err)
	}

	resp.UpdatedAt, err = ptypes.TimestampProto(rp.UpdatedAt)
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &resp, nil
}

// UpdateRoutingProfile updates the given routing-profile.
func (n *NetworkServerAPI) UpdateRoutingProfile(ctx context.Context, req *ns.UpdateRoutingProfileRequest) (*empty.Empty, error) {
	if req.RoutingProfile == nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "routing_profile must not be nil")
	}

	var rpID uuid.UUID
	copy(rpID[:], req.RoutingProfile.Id)

	rp, err := storage.GetRoutingProfile(ctx, storage.DB(), rpID)
	if err != nil {
		return nil, errToRPCError(err)
	}

	rp.ASID = req.RoutingProfile.AsId
	rp.CACert = req.RoutingProfile.CaCert
	rp.TLSCert = req.RoutingProfile.TlsCert

	if req.RoutingProfile.TlsKey != "" {
		rp.TLSKey = req.RoutingProfile.TlsKey
	}

	if rp.TLSCert == "" {
		rp.TLSKey = ""
	}

	if err := storage.UpdateRoutingProfile(ctx, storage.DB(), &rp); err != nil {
		return nil, errToRPCError(err)
	}

	return &empty.Empty{}, nil
}

// DeleteRoutingProfile deletes the routing-profile matching the given id.
func (n *NetworkServerAPI) DeleteRoutingProfile(ctx context.Context, req *ns.DeleteRoutingProfileRequest) (*empty.Empty, error) {
	var rpID uuid.UUID
	copy(rpID[:], req.Id)

	if err := storage.DeleteRoutingProfile(ctx, storage.DB(), rpID); err != nil {
		return nil, errToRPCError(err)
	}

	return &empty.Empty{}, nil
}

// CreateDeviceProfile creates the given device-profile.
// The RFRegion field will get set automatically according to the configured band.
func (n *NetworkServerAPI) CreateDeviceProfile(ctx context.Context, req *ns.CreateDeviceProfileRequest) (*ns.CreateDeviceProfileResponse, error) {
	if req.DeviceProfile == nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "device_profile must not be nil")
	}

	var dpID uuid.UUID
	copy(dpID[:], req.DeviceProfile.Id)

	var factoryPresetFreqs []int
	for _, f := range req.DeviceProfile.FactoryPresetFreqs {
		factoryPresetFreqs = append(factoryPresetFreqs, int(f))
	}

	dp := storage.DeviceProfile{
		ID:                 dpID,
		SupportsClassB:     req.DeviceProfile.SupportsClassB,
		ClassBTimeout:      int(req.DeviceProfile.ClassBTimeout),
		PingSlotPeriod:     int(req.DeviceProfile.PingSlotPeriod),
		PingSlotDR:         int(req.DeviceProfile.PingSlotDr),
		PingSlotFreq:       int(req.DeviceProfile.PingSlotFreq),
		SupportsClassC:     req.DeviceProfile.SupportsClassC,
		ClassCTimeout:      int(req.DeviceProfile.ClassCTimeout),
		MACVersion:         req.DeviceProfile.MacVersion,
		RegParamsRevision:  req.DeviceProfile.RegParamsRevision,
		RXDelay1:           int(req.DeviceProfile.RxDelay_1),
		RXDROffset1:        int(req.DeviceProfile.RxDrOffset_1),
		RXDataRate2:        int(req.DeviceProfile.RxDatarate_2),
		RXFreq2:            int(req.DeviceProfile.RxFreq_2),
		FactoryPresetFreqs: factoryPresetFreqs,
		MaxEIRP:            int(req.DeviceProfile.MaxEirp),
		MaxDutyCycle:       int(req.DeviceProfile.MaxDutyCycle),
		SupportsJoin:       req.DeviceProfile.SupportsJoin,
		Supports32bitFCnt:  req.DeviceProfile.Supports_32BitFCnt,
		RFRegion:           band.Band().Name(),
	}

	if err := storage.CreateDeviceProfile(ctx, storage.DB(), &dp); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.CreateDeviceProfileResponse{
		Id: dp.ID.Bytes(),
	}, nil
}

// GetDeviceProfile returns the device-profile matching the given id.
func (n *NetworkServerAPI) GetDeviceProfile(ctx context.Context, req *ns.GetDeviceProfileRequest) (*ns.GetDeviceProfileResponse, error) {
	var dpID uuid.UUID
	copy(dpID[:], req.Id)

	dp, err := storage.GetDeviceProfile(ctx, storage.DB(), dpID)
	if err != nil {
		return nil, errToRPCError(err)
	}

	var factoryPresetFreqs []uint32
	for _, f := range dp.FactoryPresetFreqs {
		factoryPresetFreqs = append(factoryPresetFreqs, uint32(f))
	}

	resp := ns.GetDeviceProfileResponse{
		DeviceProfile: &ns.DeviceProfile{
			Id:                 dp.ID.Bytes(),
			SupportsClassB:     dp.SupportsClassB,
			ClassBTimeout:      uint32(dp.ClassBTimeout),
			PingSlotPeriod:     uint32(dp.PingSlotPeriod),
			PingSlotDr:         uint32(dp.PingSlotDR),
			PingSlotFreq:       uint32(dp.PingSlotFreq),
			SupportsClassC:     dp.SupportsClassC,
			ClassCTimeout:      uint32(dp.ClassCTimeout),
			MacVersion:         dp.MACVersion,
			RegParamsRevision:  dp.RegParamsRevision,
			RxDelay_1:          uint32(dp.RXDelay1),
			RxDrOffset_1:       uint32(dp.RXDROffset1),
			RxDatarate_2:       uint32(dp.RXDataRate2),
			RxFreq_2:           uint32(dp.RXFreq2),
			FactoryPresetFreqs: factoryPresetFreqs,
			MaxEirp:            uint32(dp.MaxEIRP),
			MaxDutyCycle:       uint32(dp.MaxDutyCycle),
			SupportsJoin:       dp.SupportsJoin,
			RfRegion:           string(dp.RFRegion),
			Supports_32BitFCnt: dp.Supports32bitFCnt,
		},
	}

	resp.CreatedAt, err = ptypes.TimestampProto(dp.CreatedAt)
	if err != nil {
		return nil, errToRPCError(err)
	}

	resp.UpdatedAt, err = ptypes.TimestampProto(dp.UpdatedAt)
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &resp, nil
}

// UpdateDeviceProfile updates the given device-profile.
// The RFRegion field will get set automatically according to the configured band.
func (n *NetworkServerAPI) UpdateDeviceProfile(ctx context.Context, req *ns.UpdateDeviceProfileRequest) (*empty.Empty, error) {
	if req.DeviceProfile == nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "device_profile must not be nil")
	}

	var dpID uuid.UUID
	copy(dpID[:], req.DeviceProfile.Id)

	dp, err := storage.GetDeviceProfile(ctx, storage.DB(), dpID)
	if err != nil {
		return nil, errToRPCError(err)
	}

	var factoryPresetFreqs []int
	for _, f := range req.DeviceProfile.FactoryPresetFreqs {
		factoryPresetFreqs = append(factoryPresetFreqs, int(f))
	}

	dp.SupportsClassB = req.DeviceProfile.SupportsClassB
	dp.ClassBTimeout = int(req.DeviceProfile.ClassBTimeout)
	dp.PingSlotPeriod = int(req.DeviceProfile.PingSlotPeriod)
	dp.PingSlotDR = int(req.DeviceProfile.PingSlotDr)
	dp.PingSlotFreq = int(req.DeviceProfile.PingSlotFreq)
	dp.SupportsClassC = req.DeviceProfile.SupportsClassC
	dp.ClassCTimeout = int(req.DeviceProfile.ClassCTimeout)
	dp.MACVersion = req.DeviceProfile.MacVersion
	dp.RegParamsRevision = req.DeviceProfile.RegParamsRevision
	dp.RXDelay1 = int(req.DeviceProfile.RxDelay_1)
	dp.RXDROffset1 = int(req.DeviceProfile.RxDrOffset_1)
	dp.RXDataRate2 = int(req.DeviceProfile.RxDatarate_2)
	dp.RXFreq2 = int(req.DeviceProfile.RxFreq_2)
	dp.FactoryPresetFreqs = factoryPresetFreqs
	dp.MaxEIRP = int(req.DeviceProfile.MaxEirp)
	dp.MaxDutyCycle = int(req.DeviceProfile.MaxDutyCycle)
	dp.SupportsJoin = req.DeviceProfile.SupportsJoin
	dp.Supports32bitFCnt = req.DeviceProfile.Supports_32BitFCnt
	dp.RFRegion = band.Band().Name()

	if err := storage.FlushDeviceProfileCache(ctx, dp.ID); err != nil {
		return nil, errToRPCError(err)
	}

	if err := storage.UpdateDeviceProfile(ctx, storage.DB(), &dp); err != nil {
		return nil, errToRPCError(err)
	}

	return &empty.Empty{}, nil
}

// DeleteDeviceProfile deletes the device-profile matching the given id.
func (n *NetworkServerAPI) DeleteDeviceProfile(ctx context.Context, req *ns.DeleteDeviceProfileRequest) (*empty.Empty, error) {
	var dpID uuid.UUID
	copy(dpID[:], req.Id)

	if err := storage.FlushDeviceProfileCache(ctx, dpID); err != nil {
		return nil, errToRPCError(err)
	}

	if err := storage.DeleteDeviceProfile(ctx, storage.DB(), dpID); err != nil {
		return nil, errToRPCError(err)
	}

	return &empty.Empty{}, nil
}

// CreateDevice creates the given device.
func (n *NetworkServerAPI) CreateDevice(ctx context.Context, req *ns.CreateDeviceRequest) (*empty.Empty, error) {
	if req.Device == nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "device must not be nil")
	}

	var devEUI lorawan.EUI64
	var dpID, spID, rpID uuid.UUID

	copy(devEUI[:], req.Device.DevEui)
	copy(dpID[:], req.Device.DeviceProfileId)
	copy(rpID[:], req.Device.RoutingProfileId)
	copy(spID[:], req.Device.ServiceProfileId)

	d := storage.Device{
		DevEUI:            devEUI,
		DeviceProfileID:   dpID,
		ServiceProfileID:  spID,
		RoutingProfileID:  rpID,
		SkipFCntCheck:     req.Device.SkipFCntCheck,
		ReferenceAltitude: req.Device.ReferenceAltitude,
		IsDisabled:        req.Device.IsDisabled,
	}
	if err := storage.CreateDevice(ctx, storage.DB(), &d); err != nil {
		return nil, errToRPCError(err)
	}

	return &empty.Empty{}, nil
}

// GetDevice returns the device matching the given DevEUI.
func (n *NetworkServerAPI) GetDevice(ctx context.Context, req *ns.GetDeviceRequest) (*ns.GetDeviceResponse, error) {
	var devEUI lorawan.EUI64
	copy(devEUI[:], req.DevEui)

	d, err := storage.GetDevice(ctx, storage.DB(), devEUI)
	if err != nil {
		return nil, errToRPCError(err)
	}

	resp := ns.GetDeviceResponse{
		Device: &ns.Device{
			DevEui:            d.DevEUI[:],
			SkipFCntCheck:     d.SkipFCntCheck,
			DeviceProfileId:   d.DeviceProfileID[:],
			ServiceProfileId:  d.ServiceProfileID[:],
			RoutingProfileId:  d.RoutingProfileID[:],
			ReferenceAltitude: d.ReferenceAltitude,
			IsDisabled:        d.IsDisabled,
		},
	}

	resp.CreatedAt, err = ptypes.TimestampProto(d.CreatedAt)
	if err != nil {
		return nil, errToRPCError(err)
	}

	resp.UpdatedAt, err = ptypes.TimestampProto(d.UpdatedAt)
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &resp, nil
}

// UpdateDevice updates the given device.
func (n *NetworkServerAPI) UpdateDevice(ctx context.Context, req *ns.UpdateDeviceRequest) (*empty.Empty, error) {
	if req.Device == nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "device must not be nil")
	}

	var devEUI lorawan.EUI64
	var dpID, spID, rpID uuid.UUID

	copy(devEUI[:], req.Device.DevEui)
	copy(dpID[:], req.Device.DeviceProfileId)
	copy(rpID[:], req.Device.RoutingProfileId)
	copy(spID[:], req.Device.ServiceProfileId)

	d, err := storage.GetDevice(ctx, storage.DB(), devEUI)
	if err != nil {
		return nil, errToRPCError(err)
	}

	d.DeviceProfileID = dpID
	d.ServiceProfileID = spID
	d.RoutingProfileID = rpID
	d.SkipFCntCheck = req.Device.SkipFCntCheck
	d.ReferenceAltitude = req.Device.ReferenceAltitude
	d.IsDisabled = req.Device.IsDisabled

	err = storage.Transaction(func(tx sqlx.Ext) error {
		if err := storage.UpdateDevice(ctx, tx, &d); err != nil {
			return err
		}

		// if there is a device-session, set the is disabled field
		ds, err := storage.GetDeviceSession(ctx, devEUI)
		if err == nil {
			ds.IsDisabled = req.Device.IsDisabled
			return storage.SaveDeviceSession(ctx, ds)
		} else if err != storage.ErrDoesNotExist {
			return err
		}

		return nil
	})
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &empty.Empty{}, nil
}

// DeleteDevice deletes the device matching the given DevEUI.
func (n *NetworkServerAPI) DeleteDevice(ctx context.Context, req *ns.DeleteDeviceRequest) (*empty.Empty, error) {
	var devEUI lorawan.EUI64
	copy(devEUI[:], req.DevEui)

	err := storage.Transaction(func(tx sqlx.Ext) error {
		if err := storage.DeleteDevice(ctx, tx, devEUI); err != nil {
			return errToRPCError(err)
		}

		if err := storage.DeleteDeviceSession(ctx, devEUI); err != nil && err != storage.ErrDoesNotExist {
			return errToRPCError(err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return &empty.Empty{}, nil
}

// ActivateDevice activates a device (ABP).
func (n *NetworkServerAPI) ActivateDevice(ctx context.Context, req *ns.ActivateDeviceRequest) (*empty.Empty, error) {
	if req.DeviceActivation == nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "device_activation must not be nil")
	}

	var devEUI lorawan.EUI64
	var devAddr lorawan.DevAddr
	var sNwkSIntKey, fNwkSIntKey, nwkSEncKey lorawan.AES128Key

	copy(devEUI[:], req.DeviceActivation.DevEui)
	copy(devAddr[:], req.DeviceActivation.DevAddr)
	copy(sNwkSIntKey[:], req.DeviceActivation.SNwkSIntKey)
	copy(fNwkSIntKey[:], req.DeviceActivation.FNwkSIntKey)
	copy(nwkSEncKey[:], req.DeviceActivation.NwkSEncKey)

	d, err := storage.GetDevice(ctx, storage.DB(), devEUI)
	if err != nil {
		return nil, errToRPCError(err)
	}

	dp, err := storage.GetDeviceProfile(ctx, storage.DB(), d.DeviceProfileID)
	if err != nil {
		return nil, errToRPCError(err)
	}

	ds := storage.DeviceSession{
		DeviceProfileID:  d.DeviceProfileID,
		ServiceProfileID: d.ServiceProfileID,
		RoutingProfileID: d.RoutingProfileID,

		DevEUI:             devEUI,
		DevAddr:            devAddr,
		SNwkSIntKey:        sNwkSIntKey,
		FNwkSIntKey:        fNwkSIntKey,
		NwkSEncKey:         nwkSEncKey,
		FCntUp:             req.DeviceActivation.FCntUp,
		NFCntDown:          req.DeviceActivation.NFCntDown,
		AFCntDown:          req.DeviceActivation.AFCntDown,
		SkipFCntValidation: req.DeviceActivation.SkipFCntCheck || d.SkipFCntCheck,

		RXWindow: storage.RX1,

		MACVersion: dp.MACVersion,

		IsDisabled: d.IsDisabled,
	}

	// The device is never set to DeviceModeB because the device first needs to
	// aquire a Class-B beacon lock and will signal this to the network-server.
	if dp.SupportsClassC {
		d.Mode = storage.DeviceModeC
	} else {
		d.Mode = storage.DeviceModeA
	}
	if err := storage.UpdateDevice(ctx, storage.DB(), &d); err != nil {
		return nil, errToRPCError(err)
	}

	// reset the device-session to the device boot parameters
	ds.ResetToBootParameters(dp)

	if err := storage.SaveDeviceSession(ctx, ds); err != nil {
		return nil, errToRPCError(err)
	}

	if err := storage.FlushDeviceQueueForDevEUI(ctx, storage.DB(), d.DevEUI); err != nil {
		return nil, errToRPCError(err)
	}

	if err := storage.FlushMACCommandQueue(ctx, ds.DevEUI); err != nil {
		return nil, errToRPCError(err)
	}

	return &empty.Empty{}, nil
}

// DeactivateDevice de-activates a device.
func (n *NetworkServerAPI) DeactivateDevice(ctx context.Context, req *ns.DeactivateDeviceRequest) (*empty.Empty, error) {
	var devEUI lorawan.EUI64
	copy(devEUI[:], req.DevEui)

	if err := storage.DeleteDeviceSession(ctx, devEUI); err != nil {
		return nil, errToRPCError(err)
	}

	if err := storage.FlushDeviceQueueForDevEUI(ctx, storage.DB(), devEUI); err != nil {
		return nil, errToRPCError(err)
	}

	return &empty.Empty{}, nil
}

// GetDeviceActivation returns the device activation details.
func (n *NetworkServerAPI) GetDeviceActivation(ctx context.Context, req *ns.GetDeviceActivationRequest) (*ns.GetDeviceActivationResponse, error) {
	var devEUI lorawan.EUI64
	copy(devEUI[:], req.DevEui)

	ds, err := storage.GetDeviceSession(ctx, devEUI)
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.GetDeviceActivationResponse{
		DeviceActivation: &ns.DeviceActivation{
			DevEui:        ds.DevEUI[:],
			DevAddr:       ds.DevAddr[:],
			SNwkSIntKey:   ds.SNwkSIntKey[:],
			FNwkSIntKey:   ds.FNwkSIntKey[:],
			NwkSEncKey:    ds.NwkSEncKey[:],
			FCntUp:        ds.FCntUp,
			NFCntDown:     ds.NFCntDown,
			AFCntDown:     ds.AFCntDown,
			SkipFCntCheck: ds.SkipFCntValidation,
		},
	}, nil
}

// GetRandomDevAddr returns a random DevAddr.
func (n *NetworkServerAPI) GetRandomDevAddr(ctx context.Context, req *empty.Empty) (*ns.GetRandomDevAddrResponse, error) {
	devAddr, err := storage.GetRandomDevAddr(config.C.NetworkServer.NetID)
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.GetRandomDevAddrResponse{
		DevAddr: devAddr[:],
	}, nil
}

// CreateMACCommandQueueItem adds a data down MAC command to the queue.
// It replaces already enqueued mac-commands with the same CID.
func (n *NetworkServerAPI) CreateMACCommandQueueItem(ctx context.Context, req *ns.CreateMACCommandQueueItemRequest) (*empty.Empty, error) {
	var commands []lorawan.MACCommand
	var devEUI lorawan.EUI64

	copy(devEUI[:], req.DevEui)

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

	if err := storage.CreateMACCommandQueueItem(ctx, devEUI, block); err != nil {
		return nil, errToRPCError(err)
	}

	return &empty.Empty{}, nil
}

// SendProprietaryPayload send a payload using the 'Proprietary' LoRaWAN message-type.
func (n *NetworkServerAPI) SendProprietaryPayload(ctx context.Context, req *ns.SendProprietaryPayloadRequest) (*empty.Empty, error) {
	var mic lorawan.MIC
	var gwIDs []lorawan.EUI64

	copy(mic[:], req.Mic)
	for i := range req.GatewayMacs {
		var id lorawan.EUI64
		copy(id[:], req.GatewayMacs[i])
		gwIDs = append(gwIDs, id)
	}

	err := proprietarydown.Handle(ctx, req.MacPayload, mic, gwIDs, req.PolarizationInversion, int(req.Frequency), int(req.Dr))
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &empty.Empty{}, nil
}

// CreateGateway creates the given gateway.
func (n *NetworkServerAPI) CreateGateway(ctx context.Context, req *ns.CreateGatewayRequest) (*empty.Empty, error) {
	if req.Gateway == nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "gateway must not be nil")
	}

	if req.Gateway.Location == nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "gateway.location must not be nil")
	}

	gw := storage.Gateway{
		Location: storage.GPSPoint{
			Latitude:  req.Gateway.Location.Latitude,
			Longitude: req.Gateway.Location.Longitude,
		},
		Altitude: req.Gateway.Location.Altitude,
	}

	// Gateway ID
	copy(gw.GatewayID[:], req.Gateway.Id)

	// Gateway-profile ID.
	if b := req.Gateway.GatewayProfileId; len(b) != 0 {
		var gpID uuid.UUID
		copy(gpID[:], b)
		gw.GatewayProfileID = &gpID
	}

	// Routing Profile ID.
	copy(gw.RoutingProfileID[:], req.Gateway.RoutingProfileId)

	for _, board := range req.Gateway.Boards {
		var gwBoard storage.GatewayBoard

		if b := board.FpgaId; len(b) != 0 {
			var fpgaID lorawan.EUI64
			copy(fpgaID[:], b)
			gwBoard.FPGAID = &fpgaID
		}

		if b := board.FineTimestampKey; len(b) != 0 {
			var key lorawan.AES128Key
			copy(key[:], b)
			gwBoard.FineTimestampKey = &key
		}

		gw.Boards = append(gw.Boards, gwBoard)
	}

	err := storage.Transaction(func(tx sqlx.Ext) error {
		if err := storage.CreateGateway(ctx, tx, &gw); err != nil {
			return errToRPCError(err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &empty.Empty{}, nil
}

// GetGateway returns data for a particular gateway.
func (n *NetworkServerAPI) GetGateway(ctx context.Context, req *ns.GetGatewayRequest) (*ns.GetGatewayResponse, error) {
	var id lorawan.EUI64
	copy(id[:], req.Id)

	gw, err := storage.GetGateway(ctx, storage.DB(), id)
	if err != nil {
		return nil, errToRPCError(err)
	}

	resp := ns.GetGatewayResponse{
		Gateway: &ns.Gateway{
			Id:               gw.GatewayID[:],
			RoutingProfileId: gw.RoutingProfileID[:],
			Location: &common.Location{
				Latitude:  gw.Location.Latitude,
				Longitude: gw.Location.Longitude,
				Altitude:  gw.Altitude,
			},
		},
	}

	resp.CreatedAt, _ = ptypes.TimestampProto(gw.CreatedAt)
	resp.UpdatedAt, _ = ptypes.TimestampProto(gw.UpdatedAt)

	if gw.GatewayProfileID != nil {
		resp.Gateway.GatewayProfileId = gw.GatewayProfileID.Bytes()
	}

	if gw.FirstSeenAt != nil {
		resp.FirstSeenAt, _ = ptypes.TimestampProto(*gw.FirstSeenAt)
	}

	if gw.LastSeenAt != nil {
		resp.LastSeenAt, _ = ptypes.TimestampProto(*gw.LastSeenAt)
	}

	for i := range gw.Boards {
		var gwBoard ns.GatewayBoard
		if gw.Boards[i].FPGAID != nil {
			gwBoard.FpgaId = gw.Boards[i].FPGAID[:]
		}

		if gw.Boards[i].FineTimestampKey != nil {
			gwBoard.FineTimestampKey = gw.Boards[i].FineTimestampKey[:]
		}

		resp.Gateway.Boards = append(resp.Gateway.Boards, &gwBoard)
	}

	return &resp, nil
}

// UpdateGateway updates an existing gateway.
func (n *NetworkServerAPI) UpdateGateway(ctx context.Context, req *ns.UpdateGatewayRequest) (*empty.Empty, error) {
	if req.Gateway == nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "gateway must not be nil")
	}

	if req.Gateway.Location == nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "gateway.location must not be nil")
	}

	var id lorawan.EUI64
	copy(id[:], req.Gateway.Id)

	gw, err := storage.GetGateway(ctx, storage.DB(), id)
	if err != nil {
		return nil, errToRPCError(err)
	}

	// Routing Profile ID.
	copy(gw.RoutingProfileID[:], req.Gateway.RoutingProfileId)

	// Gateway-profile ID.
	if b := req.Gateway.GatewayProfileId; len(b) != 0 {
		var gpID uuid.UUID
		copy(gpID[:], b)
		gw.GatewayProfileID = &gpID
	} else {
		gw.GatewayProfileID = nil
	}

	gw.Location = storage.GPSPoint{
		Latitude:  req.Gateway.Location.Latitude,
		Longitude: req.Gateway.Location.Longitude,
	}
	gw.Altitude = req.Gateway.Location.Altitude

	gw.Boards = nil
	for _, board := range req.Gateway.Boards {
		var gwBoard storage.GatewayBoard

		if b := board.FpgaId; len(b) != 0 {
			var fpgaID lorawan.EUI64
			copy(fpgaID[:], b)
			gwBoard.FPGAID = &fpgaID
		}

		if b := board.FineTimestampKey; len(b) != 0 {
			var key lorawan.AES128Key
			copy(key[:], b)
			gwBoard.FineTimestampKey = &key
		}

		gw.Boards = append(gw.Boards, gwBoard)
	}

	if err = storage.FlushGatewayCache(ctx, gw.GatewayID); err != nil {
		return nil, errToRPCError(err)
	}

	err = storage.Transaction(func(tx sqlx.Ext) error {
		if err = storage.UpdateGateway(ctx, tx, &gw); err != nil {
			return errToRPCError(err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &empty.Empty{}, nil
}

// DeleteGateway deletes a gateway.
func (n *NetworkServerAPI) DeleteGateway(ctx context.Context, req *ns.DeleteGatewayRequest) (*empty.Empty, error) {
	var id lorawan.EUI64
	copy(id[:], req.Id)

	if err := storage.FlushGatewayCache(ctx, id); err != nil {
		return nil, errToRPCError(err)
	}

	if err := storage.DeleteGateway(ctx, storage.DB(), id); err != nil {
		return nil, errToRPCError(err)
	}

	return &empty.Empty{}, nil
}

// GenerateGatewayClientCertificate returns a TLS certificate for gateway authentication / authorization.
func (c *NetworkServerAPI) GenerateGatewayClientCertificate(ctx context.Context, req *ns.GenerateGatewayClientCertificateRequest) (*ns.GenerateGatewayClientCertificateResponse, error) {
	var id lorawan.EUI64
	copy(id[:], req.Id)

	var ca, cert, key []byte

	err := storage.Transaction(func(tx sqlx.Ext) error {
		gw, err := storage.GetGateway(ctx, tx, id)
		if err != nil {
			return err
		}

		ca, cert, key, err = gateway.GenerateClientCertificate(id)
		if err != nil {
			return err
		}

		gw.TLSCert = cert
		return storage.UpdateGateway(ctx, tx, &gw)
	})
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.GenerateGatewayClientCertificateResponse{
		TlsCert: cert,
		TlsKey:  key,
		CaCert:  ca,
	}, nil
}

// GetGatewayStats returns stats of an existing gateway.
func (n *NetworkServerAPI) GetGatewayStats(ctx context.Context, req *ns.GetGatewayStatsRequest) (*ns.GetGatewayStatsResponse, error) {
	gatewayID := helpers.GetGatewayID(req)

	start, err := ptypes.Timestamp(req.StartTimestamp)
	if err != nil {
		return nil, grpc.Errorf(codes.InvalidArgument, err.Error())
	}

	end, err := ptypes.Timestamp(req.EndTimestamp)
	if err != nil {
		return nil, grpc.Errorf(codes.InvalidArgument, err.Error())
	}

	metrics, err := storage.GetMetrics(ctx, storage.AggregationInterval(req.Interval.String()), "gw:"+gatewayID.String(), start, end)
	if err != nil {
		return nil, errToRPCError(err)
	}

	var resp ns.GetGatewayStatsResponse

	for _, m := range metrics {
		row := ns.GatewayStats{
			RxPacketsReceived:   int32(m.Metrics["rx_count"]),
			RxPacketsReceivedOk: int32(m.Metrics["rx_ok_count"]),
			TxPacketsReceived:   int32(m.Metrics["tx_count"]),
			TxPacketsEmitted:    int32(m.Metrics["tx_ok_count"]),
		}

		row.Timestamp, err = ptypes.TimestampProto(m.Time)
		if err != nil {
			return nil, errToRPCError(err)
		}

		resp.Result = append(resp.Result, &row)
	}

	return &resp, nil
}

// StreamFrameLogsForGateway returns a stream of frames seen by the given gateway.
func (n *NetworkServerAPI) StreamFrameLogsForGateway(req *ns.StreamFrameLogsForGatewayRequest, srv ns.NetworkServerService_StreamFrameLogsForGatewayServer) error {
	frameLogChan := make(chan framelog.FrameLog)
	var id lorawan.EUI64
	copy(id[:], req.GatewayId)

	go func() {
		err := framelog.GetFrameLogForGateway(srv.Context(), id, frameLogChan)
		if err != nil {
			log.WithError(err).Error("get frame-log for gateway error")
		}
		close(frameLogChan)
	}()

	for fl := range frameLogChan {
		resp := ns.StreamFrameLogsForGatewayResponse{}

		if fl.UplinkFrame != nil {
			resp.Frame = &ns.StreamFrameLogsForGatewayResponse_UplinkFrameSet{
				UplinkFrameSet: fl.UplinkFrame,
			}
		}

		if fl.DownlinkFrame != nil {
			resp.Frame = &ns.StreamFrameLogsForGatewayResponse_DownlinkFrame{
				DownlinkFrame: fl.DownlinkFrame,
			}
		}

		if err := srv.Send(&resp); err != nil {
			log.WithError(err).Error("error sending frame-log response")
		}
	}

	return nil
}

// StreamFrameLogsForDevice returns a stream of frames seen by the given device.
func (n *NetworkServerAPI) StreamFrameLogsForDevice(req *ns.StreamFrameLogsForDeviceRequest, srv ns.NetworkServerService_StreamFrameLogsForDeviceServer) error {
	frameLogChan := make(chan framelog.FrameLog)
	var devEUI lorawan.EUI64
	copy(devEUI[:], req.DevEui)

	go func() {
		err := framelog.GetFrameLogForDevice(srv.Context(), devEUI, frameLogChan)
		if err != nil {
			log.WithError(err).Error("get frame-log for device error")
		}
		close(frameLogChan)
	}()

	for fl := range frameLogChan {
		resp := ns.StreamFrameLogsForDeviceResponse{}

		if fl.UplinkFrame != nil {
			resp.Frame = &ns.StreamFrameLogsForDeviceResponse_UplinkFrameSet{
				UplinkFrameSet: fl.UplinkFrame,
			}
		}

		if fl.DownlinkFrame != nil {
			resp.Frame = &ns.StreamFrameLogsForDeviceResponse_DownlinkFrame{
				DownlinkFrame: fl.DownlinkFrame,
			}
		}

		if err := srv.Send(&resp); err != nil {
			log.WithError(err).Error("error sending frame-log response")
		}
	}

	return nil
}

// CreateGatewayProfile creates the given gateway-profile.
func (n *NetworkServerAPI) CreateGatewayProfile(ctx context.Context, req *ns.CreateGatewayProfileRequest) (*ns.CreateGatewayProfileResponse, error) {
	if req.GatewayProfile == nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "gateway_profile must not be nil")
	}

	var gpID uuid.UUID
	copy(gpID[:], req.GatewayProfile.Id)

	gc := storage.GatewayProfile{
		ID: gpID,
	}

	if req.GatewayProfile.StatsInterval != nil {
		statsInterval, err := ptypes.Duration(req.GatewayProfile.StatsInterval)
		if err != nil {
			return nil, grpc.Errorf(codes.InvalidArgument, "stats_interval: %s", err)
		}

		gc.StatsInterval = statsInterval
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
		case common.Modulation_FSK:
			c.Modulation = storage.ModulationFSK
		default:
			c.Modulation = storage.ModulationLoRa
		}

		for _, sf := range ec.SpreadingFactors {
			c.SpreadingFactors = append(c.SpreadingFactors, int64(sf))
		}

		gc.ExtraChannels = append(gc.ExtraChannels, c)
	}

	err := storage.Transaction(func(tx sqlx.Ext) error {
		return storage.CreateGatewayProfile(ctx, tx, &gc)
	})
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.CreateGatewayProfileResponse{Id: gc.ID.Bytes()}, nil
}

// GetGatewayProfile returns the gateway-profile given an id.
func (n *NetworkServerAPI) GetGatewayProfile(ctx context.Context, req *ns.GetGatewayProfileRequest) (*ns.GetGatewayProfileResponse, error) {
	var gpID uuid.UUID
	copy(gpID[:], req.Id)

	gc, err := storage.GetGatewayProfile(ctx, storage.DB(), gpID)
	if err != nil {
		return nil, errToRPCError(err)
	}

	out := ns.GetGatewayProfileResponse{
		GatewayProfile: &ns.GatewayProfile{
			Id:            gc.ID.Bytes(),
			StatsInterval: ptypes.DurationProto(gc.StatsInterval),
		},
	}

	out.CreatedAt, err = ptypes.TimestampProto(gc.CreatedAt)
	if err != nil {
		return nil, errToRPCError(err)
	}

	out.UpdatedAt, err = ptypes.TimestampProto(gc.UpdatedAt)
	if err != nil {
		return nil, errToRPCError(err)
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
			c.Modulation = common.Modulation_FSK
		default:
			c.Modulation = common.Modulation_LORA
		}

		for _, sf := range ec.SpreadingFactors {
			c.SpreadingFactors = append(c.SpreadingFactors, uint32(sf))
		}

		out.GatewayProfile.ExtraChannels = append(out.GatewayProfile.ExtraChannels, &c)
	}

	return &out, nil
}

// UpdateGatewayProfile updates the given gateway-profile.
func (n *NetworkServerAPI) UpdateGatewayProfile(ctx context.Context, req *ns.UpdateGatewayProfileRequest) (*empty.Empty, error) {
	if req.GatewayProfile == nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "gateway_profile must not be nil")
	}

	var gpID uuid.UUID
	copy(gpID[:], req.GatewayProfile.Id)

	gc, err := storage.GetGatewayProfile(ctx, storage.DB(), gpID)
	if err != nil {
		return nil, errToRPCError(err)
	}

	gc.StatsInterval = 0
	if req.GatewayProfile.StatsInterval != nil {
		statsInterval, err := ptypes.Duration(req.GatewayProfile.StatsInterval)
		if err != nil {
			return nil, grpc.Errorf(codes.InvalidArgument, "stats_interval: %s", err)
		}
		gc.StatsInterval = statsInterval
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
		case common.Modulation_FSK:
			c.Modulation = storage.ModulationFSK
		default:
			c.Modulation = storage.ModulationLoRa
		}

		for _, sf := range ec.SpreadingFactors {
			c.SpreadingFactors = append(c.SpreadingFactors, int64(sf))
		}

		gc.ExtraChannels = append(gc.ExtraChannels, c)
	}

	err = storage.Transaction(func(tx sqlx.Ext) error {
		return storage.UpdateGatewayProfile(ctx, tx, &gc)
	})
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &empty.Empty{}, nil
}

// DeleteGatewayProfile deletes the gateway-profile matching a given id.
func (n *NetworkServerAPI) DeleteGatewayProfile(ctx context.Context, req *ns.DeleteGatewayProfileRequest) (*empty.Empty, error) {
	var gpID uuid.UUID
	copy(gpID[:], req.Id)

	if err := storage.DeleteGatewayProfile(ctx, storage.DB(), gpID); err != nil {
		return nil, errToRPCError(err)
	}

	return &empty.Empty{}, nil
}

// CreateDeviceQueueItem creates the given device-queue item.
func (n *NetworkServerAPI) CreateDeviceQueueItem(ctx context.Context, req *ns.CreateDeviceQueueItemRequest) (*empty.Empty, error) {
	if req.Item == nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "item must not be nil")
	}

	var devEUI lorawan.EUI64
	copy(devEUI[:], req.Item.DevEui)

	d, err := storage.GetDevice(ctx, storage.DB(), devEUI)
	if err != nil {
		return nil, errToRPCError(err)
	}

	dp, err := storage.GetAndCacheDeviceProfile(ctx, storage.DB(), d.DeviceProfileID)
	if err != nil {
		return nil, errToRPCError(err)
	}

	ds, err := storage.GetDeviceSession(ctx, d.DevEUI)
	if err != nil {
		return nil, errToRPCError(err)
	}

	var devAddr lorawan.DevAddr
	copy(devAddr[:], req.Item.DevAddr)

	if (devAddr != lorawan.DevAddr{0, 0, 0, 0} && ds.DevAddr != devAddr) {
		return nil, grpc.Errorf(codes.InvalidArgument, "device security-context out of sync")
	}

	qi := storage.DeviceQueueItem{
		DevAddr:    devAddr,
		DevEUI:     d.DevEUI,
		FRMPayload: req.Item.FrmPayload,
		FCnt:       req.Item.FCnt,
		FPort:      uint8(req.Item.FPort),
		Confirmed:  req.Item.Confirmed,
	}

	// When the device is operating in Class-B and has a beacon lock, calculate
	// the next ping-slot.
	if dp.SupportsClassB {
		// check if device is currently active and is operating in Class-B mode
		if err == nil && ds.BeaconLocked {
			scheduleAfterGPSEpochTS, err := storage.GetMaxEmitAtTimeSinceGPSEpochForDevEUI(ctx, storage.DB(), d.DevEUI)
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

	err = storage.CreateDeviceQueueItem(ctx, storage.DB(), &qi)
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &empty.Empty{}, nil
}

// FlushDeviceQueueForDevEUI flushes the device-queue for the given DevEUI.
func (n *NetworkServerAPI) FlushDeviceQueueForDevEUI(ctx context.Context, req *ns.FlushDeviceQueueForDevEUIRequest) (*empty.Empty, error) {
	var devEUI lorawan.EUI64
	copy(devEUI[:], req.DevEui)

	err := storage.FlushDeviceQueueForDevEUI(ctx, storage.DB(), devEUI)
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &empty.Empty{}, nil
}

// GetDeviceQueueItemsForDevEUI returns all device-queue items for the given DevEUI.
func (n *NetworkServerAPI) GetDeviceQueueItemsForDevEUI(ctx context.Context, req *ns.GetDeviceQueueItemsForDevEUIRequest) (*ns.GetDeviceQueueItemsForDevEUIResponse, error) {
	var devEUI lorawan.EUI64
	copy(devEUI[:], req.DevEui)

	var out ns.GetDeviceQueueItemsForDevEUIResponse
	if req.CountOnly {
		count, err := storage.GetDeviceQueueItemCountForDevEUI(ctx, storage.DB(), devEUI)
		if err != nil {
			return nil, errToRPCError(err)
		}
		out.TotalCount = uint32(count)
	} else {
		items, err := storage.GetDeviceQueueItemsForDevEUI(ctx, storage.DB(), devEUI)
		if err != nil {
			return nil, errToRPCError(err)
		}

		out.TotalCount = uint32(len(items))

		for i := range items {
			qi := ns.DeviceQueueItem{
				DevAddr:    items[i].DevAddr[:],
				DevEui:     items[i].DevEUI[:],
				FrmPayload: items[i].FRMPayload,
				FCnt:       items[i].FCnt,
				FPort:      uint32(items[i].FPort),
				Confirmed:  items[i].Confirmed,
			}

			out.Items = append(out.Items, &qi)
		}
	}

	return &out, nil
}

// GetNextDownlinkFCntForDevEUI returns the next FCnt that must be used.
// This also takes device-queue items for the given DevEUI into consideration.
// In case the device is not activated, this will return an error as no
// device-session exists.
func (n *NetworkServerAPI) GetNextDownlinkFCntForDevEUI(ctx context.Context, req *ns.GetNextDownlinkFCntForDevEUIRequest) (*ns.GetNextDownlinkFCntForDevEUIResponse, error) {
	var devEUI lorawan.EUI64
	var resp ns.GetNextDownlinkFCntForDevEUIResponse

	copy(devEUI[:], req.DevEui)

	ds, err := storage.GetDeviceSession(ctx, devEUI)
	if err != nil {
		return nil, errToRPCError(err)
	}

	if ds.GetMACVersion() == lorawan.LoRaWAN1_0 {
		resp.FCnt = ds.NFCntDown
	} else {
		resp.FCnt = ds.AFCntDown
	}

	items, err := storage.GetDeviceQueueItemsForDevEUI(ctx, storage.DB(), devEUI)
	if err != nil {
		return nil, errToRPCError(err)
	}
	if count := len(items); count != 0 {
		resp.FCnt = items[count-1].FCnt + 1 // we want the next usable frame-counter
	}

	return &resp, nil
}

// CreateMulticastGroup creates the given multicast-group.
func (n *NetworkServerAPI) CreateMulticastGroup(ctx context.Context, req *ns.CreateMulticastGroupRequest) (*ns.CreateMulticastGroupResponse, error) {
	if req.MulticastGroup == nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "multicast_group must not be nil")
	}

	mg := storage.MulticastGroup{
		FCnt:           req.MulticastGroup.FCnt,
		DR:             int(req.MulticastGroup.Dr),
		Frequency:      int(req.MulticastGroup.Frequency),
		PingSlotPeriod: int(req.MulticastGroup.PingSlotPeriod),
	}

	switch req.MulticastGroup.GroupType {
	case ns.MulticastGroupType_CLASS_B:
		mg.GroupType = storage.MulticastGroupB
	case ns.MulticastGroupType_CLASS_C:
		mg.GroupType = storage.MulticastGroupC
	default:
		return nil, grpc.Errorf(codes.InvalidArgument, "invalid group_type")
	}

	copy(mg.ID[:], req.MulticastGroup.Id)
	copy(mg.MCAddr[:], req.MulticastGroup.McAddr)
	copy(mg.MCNwkSKey[:], req.MulticastGroup.McNwkSKey)
	copy(mg.ServiceProfileID[:], req.MulticastGroup.ServiceProfileId)
	copy(mg.RoutingProfileID[:], req.MulticastGroup.RoutingProfileId)

	if err := storage.CreateMulticastGroup(ctx, storage.DB(), &mg); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.CreateMulticastGroupResponse{
		Id: mg.ID.Bytes(),
	}, nil
}

// GetMulticastGroup returns the multicast-group given an id.
func (n *NetworkServerAPI) GetMulticastGroup(ctx context.Context, req *ns.GetMulticastGroupRequest) (*ns.GetMulticastGroupResponse, error) {
	var mgID uuid.UUID
	copy(mgID[:], req.Id)

	mg, err := storage.GetMulticastGroup(ctx, storage.DB(), mgID, false)
	if err != nil {
		return nil, errToRPCError(err)
	}

	resp := ns.GetMulticastGroupResponse{
		MulticastGroup: &ns.MulticastGroup{
			Id:               mg.ID.Bytes(),
			McAddr:           mg.MCAddr[:],
			McNwkSKey:        mg.MCNwkSKey[:],
			FCnt:             mg.FCnt,
			Dr:               uint32(mg.DR),
			Frequency:        uint32(mg.Frequency),
			PingSlotPeriod:   uint32(mg.PingSlotPeriod),
			ServiceProfileId: mg.ServiceProfileID.Bytes(),
			RoutingProfileId: mg.RoutingProfileID.Bytes(),
		},
	}

	switch mg.GroupType {
	case storage.MulticastGroupB:
		resp.MulticastGroup.GroupType = ns.MulticastGroupType_CLASS_B
	case storage.MulticastGroupC:
		resp.MulticastGroup.GroupType = ns.MulticastGroupType_CLASS_C
	default:
		return nil, grpc.Errorf(codes.Internal, "invalid group-type: %s", mg.GroupType)
	}

	resp.CreatedAt, err = ptypes.TimestampProto(mg.CreatedAt)
	if err != nil {
		return nil, errToRPCError(err)
	}

	resp.UpdatedAt, err = ptypes.TimestampProto(mg.UpdatedAt)
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &resp, nil
}

// UpdateMulticastGroup updates the given multicast-group.
func (n *NetworkServerAPI) UpdateMulticastGroup(ctx context.Context, req *ns.UpdateMulticastGroupRequest) (*empty.Empty, error) {
	if req.MulticastGroup == nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "multicast_group must not be nil")
	}

	var mgID uuid.UUID
	copy(mgID[:], req.MulticastGroup.Id)

	err := storage.Transaction(func(tx sqlx.Ext) error {
		mg, err := storage.GetMulticastGroup(ctx, tx, mgID, true)
		if err != nil {
			return errToRPCError(err)
		}

		copy(mg.MCAddr[:], req.MulticastGroup.McAddr)
		copy(mg.MCNwkSKey[:], req.MulticastGroup.McNwkSKey)
		copy(mg.ServiceProfileID[:], req.MulticastGroup.ServiceProfileId)
		copy(mg.RoutingProfileID[:], req.MulticastGroup.RoutingProfileId)
		mg.FCnt = req.MulticastGroup.FCnt
		mg.DR = int(req.MulticastGroup.Dr)
		mg.Frequency = int(req.MulticastGroup.Frequency)
		mg.PingSlotPeriod = int(req.MulticastGroup.PingSlotPeriod)

		switch req.MulticastGroup.GroupType {
		case ns.MulticastGroupType_CLASS_B:
			mg.GroupType = storage.MulticastGroupB
		case ns.MulticastGroupType_CLASS_C:
			mg.GroupType = storage.MulticastGroupC
		default:
			return grpc.Errorf(codes.InvalidArgument, "invalid group_type")
		}

		if err := storage.UpdateMulticastGroup(ctx, tx, &mg); err != nil {
			return errToRPCError(err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return &empty.Empty{}, nil
}

// DeleteMulticastGroup deletes a multicast-group given an id.
func (n *NetworkServerAPI) DeleteMulticastGroup(ctx context.Context, req *ns.DeleteMulticastGroupRequest) (*empty.Empty, error) {
	var mgID uuid.UUID
	copy(mgID[:], req.Id)

	if err := storage.DeleteMulticastGroup(ctx, storage.DB(), mgID); err != nil {
		return nil, errToRPCError(err)
	}

	return &empty.Empty{}, nil
}

// AddDeviceToMulticastGroup adds the given device to the given multicast-group.
func (n *NetworkServerAPI) AddDeviceToMulticastGroup(ctx context.Context, req *ns.AddDeviceToMulticastGroupRequest) (*empty.Empty, error) {
	var devEUI lorawan.EUI64
	var mgID uuid.UUID
	copy(devEUI[:], req.DevEui)
	copy(mgID[:], req.MulticastGroupId)

	if err := storage.AddDeviceToMulticastGroup(ctx, storage.DB(), devEUI, mgID); err != nil {
		return nil, errToRPCError(err)
	}

	return &empty.Empty{}, nil
}

// RemoveDeviceFromMulticastGroup removes the given device from the given multicast-group.
func (n *NetworkServerAPI) RemoveDeviceFromMulticastGroup(ctx context.Context, req *ns.RemoveDeviceFromMulticastGroupRequest) (*empty.Empty, error) {
	var devEUI lorawan.EUI64
	var mgID uuid.UUID
	copy(devEUI[:], req.DevEui)
	copy(mgID[:], req.MulticastGroupId)

	if err := storage.RemoveDeviceFromMulticastGroup(ctx, storage.DB(), devEUI, mgID); err != nil {
		return nil, errToRPCError(err)
	}

	return &empty.Empty{}, nil
}

// EnqueueMulticastQueueItem creates the given multicast queue-item.
func (n *NetworkServerAPI) EnqueueMulticastQueueItem(ctx context.Context, req *ns.EnqueueMulticastQueueItemRequest) (*empty.Empty, error) {
	if req.MulticastQueueItem == nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "multicast_queue_item must not be nil")
	}

	var mgID uuid.UUID
	copy(mgID[:], req.MulticastQueueItem.MulticastGroupId)

	qi := storage.MulticastQueueItem{
		MulticastGroupID: mgID,
		FCnt:             req.MulticastQueueItem.FCnt,
		FPort:            uint8(req.MulticastQueueItem.FPort),
		FRMPayload:       req.MulticastQueueItem.FrmPayload,
	}

	err := storage.Transaction(func(tx sqlx.Ext) error {
		return multicast.EnqueueQueueItem(ctx, tx, qi)
	})
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &empty.Empty{}, nil
}

// FlushMulticastQueueForMulticastGroup flushes the multicast device-queue given a multicast-group id.
func (n *NetworkServerAPI) FlushMulticastQueueForMulticastGroup(ctx context.Context, req *ns.FlushMulticastQueueForMulticastGroupRequest) (*empty.Empty, error) {
	var mgID uuid.UUID
	copy(mgID[:], req.MulticastGroupId)

	if err := storage.FlushMulticastQueueForMulticastGroup(ctx, storage.DB(), mgID); err != nil {
		return nil, errToRPCError(err)
	}

	return &empty.Empty{}, nil
}

// GetMulticastQueueItemsForMulticastGroup returns the queue-items given a multicast-group id.
func (n *NetworkServerAPI) GetMulticastQueueItemsForMulticastGroup(ctx context.Context, req *ns.GetMulticastQueueItemsForMulticastGroupRequest) (*ns.GetMulticastQueueItemsForMulticastGroupResponse, error) {
	var mgID uuid.UUID
	copy(mgID[:], req.MulticastGroupId)

	items, err := storage.GetMulticastQueueItemsForMulticastGroup(ctx, storage.DB(), mgID)
	if err != nil {
		return nil, errToRPCError(err)
	}

	counterSeen := make(map[uint32]struct{})
	var out ns.GetMulticastQueueItemsForMulticastGroupResponse
	for i := range items {
		// we need to de-duplicate the queue-items as they are created per-gateway
		if _, seen := counterSeen[items[i].FCnt]; seen {
			continue
		}

		qi := ns.MulticastQueueItem{
			MulticastGroupId: items[i].MulticastGroupID.Bytes(),
			FrmPayload:       items[i].FRMPayload,
			FCnt:             items[i].FCnt,
			FPort:            uint32(items[i].FPort),
		}
		counterSeen[items[i].FCnt] = struct{}{}
		out.MulticastQueueItems = append(out.MulticastQueueItems, &qi)
	}

	return &out, nil
}

// GetVersion returns the ChirpStack Network Server version.
func (n *NetworkServerAPI) GetVersion(ctx context.Context, req *empty.Empty) (*ns.GetVersionResponse, error) {
	region, ok := map[string]common.Region{
		common.Region_AS923.String(): common.Region_AS923,
		common.Region_AU915.String(): common.Region_AU915,
		common.Region_CN470.String(): common.Region_CN470,
		common.Region_CN779.String(): common.Region_CN779,
		common.Region_EU433.String(): common.Region_EU433,
		common.Region_EU868.String(): common.Region_EU868,
		common.Region_IN865.String(): common.Region_IN865,
		common.Region_KR920.String(): common.Region_KR920,
		common.Region_RU864.String(): common.Region_RU864,
		common.Region_US915.String(): common.Region_US915,
	}[band.Band().Name()]

	if !ok {
		log.WithFields(log.Fields{
			"band_name": band.Band().Name(),
		}).Warning("unknown band to common name mapping")
	}

	return &ns.GetVersionResponse{
		Region:  region,
		Version: config.Version,
	}, nil
}
