//go:generate protoc -I/usr/local/include -I=. -I=$GOPATH/src --go_out=plugins=grpc:. gateway_state.proto

package storage

import (
	"fmt"
	"time"

	proto "github.com/golang/protobuf/proto"
	"github.com/gomodule/redigo/redis"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/helpers"
	"github.com/brocaar/lorawan"
)

const (
	gatewayStateTempl = "lora:ns:gw:state:%s"
)

// SaveGatewayState saves the given gateway-state.
func SaveGatewayState(p *redis.Pool, gs GatewayState) error {
	b, err := proto.Marshal(&gs)
	if err != nil {
		return errors.Wrap(err, "protobuf encode error")
	}

	gatewayID := helpers.GetGatewayID(&gs)

	c := p.Get()
	defer c.Close()
	exp := int64(config.C.NetworkServer.DeviceSessionTTL) / int64(time.Millisecond)
	_, err = c.Do("PSETEX", fmt.Sprintf(gatewayStateTempl, gatewayID), exp, b)
	if err != nil {
		return errors.Wrap(err, "psetex error")
	}

	log.WithFields(log.Fields{
		"gateway_id": gatewayID,
	}).Info("gateway-state updated")

	return nil
}

// GetGatewayState returns the gateway-state for the given gateway ID.
func GetGatewayState(p *redis.Pool, gatewayID lorawan.EUI64) (GatewayState, error) {
	var state GatewayState

	c := p.Get()
	defer c.Close()

	val, err := redis.Bytes(c.Do("GET", fmt.Sprintf(gatewayStateTempl, gatewayID)))
	if err != nil {
		if err == redis.ErrNil {
			return GatewayState{}, ErrDoesNotExist
		}
		return GatewayState{}, errors.Wrap(err, "get error")
	}

	err = proto.Unmarshal(val, &state)
	if err != nil {
		return GatewayState{}, errors.Wrap(err, "protobuf unmarshal error")
	}

	return state, nil
}

// DeleteGatewayState deletes the gateway-state for the given gateway ID.
func DeleteGatewayState(p *redis.Pool, gatewayID lorawan.EUI64) error {
	c := p.Get()
	defer c.Close()

	val, err := redis.Int(c.Do("DEL", fmt.Sprintf(gatewayStateTempl, gatewayID)))
	if err != nil {
		return errors.Wrap(err, "delete error")
	}
	if val == 0 {
		return ErrDoesNotExist
	}

	log.WithFields(log.Fields{
		"gateway_id": gatewayID,
	}).Info("gateway-state deleted")
	return nil
}
