package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/gofrs/uuid"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/chirpstack-network-server/v3/internal/logging"
	"github.com/brocaar/lorawan"
)

const (
	prDevAddrKeyTempl       = "lora:ns:pr:devaddr:%s" // pointer from DevAddr to set of session IDs (DevAddr are not guaranteed to be unique)
	prDevEUIKeyTempl        = "lora:ns:pr:deveui:%s"  // pointer from DevEUI to set of session IDs (PRStartAns DevEUI is optional, so it can't be used as main identifier)
	prDeviceSessionKeyTempl = "lora:ns:pr:sess:%s"
)

// PassiveRoamingDeviceSession defines the passive-roaming session.
type PassiveRoamingDeviceSession struct {
	SessionID   uuid.UUID
	NetID       lorawan.NetID
	DevAddr     lorawan.DevAddr
	DevEUI      lorawan.EUI64
	LoRaWAN11   bool
	FNwkSIntKey lorawan.AES128Key
	Lifetime    time.Time
	FCntUp      uint32
	ValidateMIC bool
}

// SavePassiveRoamingDeviceSession saves the passive-roaming device-session.
func SavePassiveRoamingDeviceSession(ctx context.Context, ds *PassiveRoamingDeviceSession) error {
	lifetime := ds.Lifetime.Sub(time.Now())
	if lifetime <= 0 {
		log.WithFields(log.Fields{
			"dev_eui":    ds.DevEUI,
			"dev_addr":   ds.DevAddr,
			"session_id": ds.SessionID,
			"ctx_id":     ctx.Value(logging.ContextIDKey),
			"ttl":        lifetime,
		}).Debug("storage: not saving passing-roaming session, lifetime expired")
		return nil
	}

	if ds.SessionID == uuid.Nil {
		id, err := uuid.NewV4()
		if err != nil {
			return errors.Wrap(err, "new uuid v4 error")
		}
		ds.SessionID = id
	}

	devAddrKey := GetRedisKey(prDevAddrKeyTempl, ds.DevAddr)
	devEUIKey := GetRedisKey(prDevEUIKeyTempl, ds.DevEUI)
	sessKey := GetRedisKey(prDeviceSessionKeyTempl, ds.SessionID)

	dsPB, err := passiveRoamingDeviceSessionToPB(ds)
	if err != nil {
		return errors.Wrap(err, "to protobuf error")
	}

	b, err := proto.Marshal(dsPB)
	if err != nil {
		return errors.Wrap(err, "protobuf marshal error")
	}

	// We need to store a pointer from both the DevAddr and DevEUI to the
	// passive-roaming device-session ID. This is needed:
	//  * Because the DevAddr is not guaranteed to be unique
	//  * Because the DevEUI might not be given (thus is also not guaranteed
	//    to be an unique identifier).
	//
	// But:
	//  * We need to be able to lookup the session using the DevAddr (potentially
	//    using the MIC validation).
	//  * We need to be able to stop a passive-roaming session given a DevEUI.
	pipe := RedisClient().TxPipeline()
	pipe.SAdd(ctx, devAddrKey, ds.SessionID[:])
	pipe.SAdd(ctx, devEUIKey, ds.SessionID[:])
	pipe.PExpire(ctx, devAddrKey, deviceSessionTTL)
	pipe.PExpire(ctx, devEUIKey, deviceSessionTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return errors.Wrap(err, "exec error")
	}

	err = RedisClient().Set(ctx, sessKey, b, lifetime).Err()
	if err != nil {
		return errors.Wrap(err, "set error")
	}

	log.WithFields(log.Fields{
		"dev_eui":    ds.DevEUI,
		"dev_addr":   ds.DevAddr,
		"session_id": ds.SessionID,
		"ctx_id":     ctx.Value(logging.ContextIDKey),
		"ttl":        lifetime,
	}).Info("storage: passive-roaming device-session saved")

	return nil
}

// GetPassiveRoamingDeviceSessionsForPHYPayload returns the passive-roaming
// device-sessions matching the given PHYPayload.
func GetPassiveRoamingDeviceSessionsForPHYPayload(ctx context.Context, phy lorawan.PHYPayload) ([]PassiveRoamingDeviceSession, error) {
	macPL, ok := phy.MACPayload.(*lorawan.MACPayload)
	if !ok {
		return nil, fmt.Errorf("expected *lorawan.MACPayload, got: %T", phy.MACPayload)
	}
	originalFCnt := macPL.FHDR.FCnt

	deviceSessions, err := GetPassiveRoamingDeviceSessionsForDevAddr(ctx, macPL.FHDR.DevAddr)
	if err != nil {
		return nil, err
	}

	var out []PassiveRoamingDeviceSession

	for _, ds := range deviceSessions {
		// We will not validate the MIC.
		if !ds.ValidateMIC {
			out = append(out, ds)
			continue
		}

		// Restore the original frame-counter.
		macPL.FHDR.FCnt = originalFCnt

		// Get the full 32bit frame-counter
		macPL.FHDR.FCnt = GetFullFCntUp(ds.FCntUp, macPL.FHDR.FCnt)

		if ds.LoRaWAN11 {
			ok, err = phy.ValidateUplinkDataMICF(ds.FNwkSIntKey)
		} else {
			ok, err = phy.ValidateUplinkDataMIC(lorawan.LoRaWAN1_0, 0, 0, 0, ds.FNwkSIntKey, ds.FNwkSIntKey)
		}

		// This should really not happen, so it is safe to return the error
		// instead of trying the next (possible) session.
		if err != nil {
			return nil, errors.Wrap(err, "validate mic error")
		}

		if ok {
			out = append(out, ds)
		}
	}

	return out, nil
}

// GetPassiveRoamingDeviceSessionsForDevAddr returns a slice of passive-roaming
// device-session matching the given DevAddr. When no sessions match, an empty
// slice is returned.
func GetPassiveRoamingDeviceSessionsForDevAddr(ctx context.Context, devAddr lorawan.DevAddr) ([]PassiveRoamingDeviceSession, error) {
	var items []PassiveRoamingDeviceSession

	ids, err := GetPassiveRoamingIDsForDevAddr(ctx, devAddr)
	if err != nil {
		return nil, err
	}

	for _, id := range ids {
		ds, err := GetPassiveRoamingDeviceSession(ctx, id)
		if err != nil {
			log.WithError(err).WithFields(log.Fields{
				"dev_addr": devAddr,
				"id":       id,
				"ctx_id":   ctx.Value(logging.ContextIDKey),
			}).Warning("storage: get passive-roaming device-session error")
			continue
		}

		items = append(items, ds)
	}

	return items, nil
}

// GetPassiveRoamingIDsForDevAddr returns the passive-roaming session IDs for
// the given DevAddr.
func GetPassiveRoamingIDsForDevAddr(ctx context.Context, devAddr lorawan.DevAddr) ([]uuid.UUID, error) {
	key := GetRedisKey(prDevAddrKeyTempl, devAddr)

	val, err := RedisClient().SMembers(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, errors.Wrap(err, "get passive-roaming session ids for devaddr error")
	}

	var out []uuid.UUID
	for i := range val {
		var id uuid.UUID
		copy(id[:], []byte(val[i]))
		out = append(out, id)
	}

	return out, nil
}

// GetPassiveRoamingDeviceSession returns the passive-roaming device-session.
func GetPassiveRoamingDeviceSession(ctx context.Context, id uuid.UUID) (PassiveRoamingDeviceSession, error) {
	key := GetRedisKey(prDeviceSessionKeyTempl, id)
	var dsPB PassiveRoamingDeviceSessionPB

	val, err := RedisClient().Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return PassiveRoamingDeviceSession{}, ErrDoesNotExist
		}
		return PassiveRoamingDeviceSession{}, errors.Wrap(err, "get error")
	}

	err = proto.Unmarshal(val, &dsPB)
	if err != nil {
		return PassiveRoamingDeviceSession{}, errors.Wrap(err, "unmarshal protobuf error")
	}

	return passiveRoamingDeviceSessionFromPB(&dsPB)
}

func passiveRoamingDeviceSessionToPB(ds *PassiveRoamingDeviceSession) (*PassiveRoamingDeviceSessionPB, error) {
	timePB, err := ptypes.TimestampProto(ds.Lifetime)
	if err != nil {
		return nil, errors.Wrap(err, "timestamp proto error")
	}

	return &PassiveRoamingDeviceSessionPB{
		SessionId:   ds.SessionID[:],
		NetId:       ds.NetID[:],
		DevAddr:     ds.DevAddr[:],
		DevEui:      ds.DevEUI[:],
		Lorawan_1_1: ds.LoRaWAN11,
		FNwkSIntKey: ds.FNwkSIntKey[:],
		Lifetime:    timePB,
		FCntUp:      ds.FCntUp,
		ValidateMic: ds.ValidateMIC,
	}, nil
}

func passiveRoamingDeviceSessionFromPB(dsPB *PassiveRoamingDeviceSessionPB) (PassiveRoamingDeviceSession, error) {
	ts, err := ptypes.Timestamp(dsPB.Lifetime)
	if err != nil {
		return PassiveRoamingDeviceSession{}, errors.Wrap(err, "timestamp error")
	}

	ds := PassiveRoamingDeviceSession{
		LoRaWAN11:   dsPB.Lorawan_1_1,
		Lifetime:    ts,
		FCntUp:      dsPB.FCntUp,
		ValidateMIC: dsPB.ValidateMic,
	}

	copy(ds.SessionID[:], dsPB.SessionId)
	copy(ds.NetID[:], dsPB.NetId)
	copy(ds.DevAddr[:], dsPB.DevAddr)
	copy(ds.DevEUI[:], dsPB.DevEui)
	copy(ds.FNwkSIntKey[:], dsPB.FNwkSIntKey)

	return ds, nil
}
