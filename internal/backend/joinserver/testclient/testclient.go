package testclient

import (
	"context"

	"github.com/brocaar/chirpstack-network-server/internal/backend/joinserver"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/backend"
)

// JoinServerPool is a join-server pool for testing.
type JoinServerPool struct {
	Client     joinserver.Client
	GetJoinEUI lorawan.EUI64
}

// NewJoinServerPool create a join-server pool for testing.
func NewJoinServerPool(client joinserver.Client) joinserver.Pool {
	return &JoinServerPool{
		Client: client,
	}
}

// Get method.
func (p *JoinServerPool) Get(joinEUI lorawan.EUI64) (joinserver.Client, error) {
	p.GetJoinEUI = joinEUI
	return p.Client, nil
}

// JoinServerClient is a join-server client for testing.
type JoinServerClient struct {
	JoinReqPayloadChan   chan backend.JoinReqPayload
	RejoinReqPayloadChan chan backend.RejoinReqPayload
	JoinReqError         error
	RejoinReqError       error
	JoinAnsPayload       backend.JoinAnsPayload
	RejoinAnsPayload     backend.RejoinAnsPayload
}

// NewJoinServerClient creates a new join-server client.
func NewJoinServerClient() *JoinServerClient {
	return &JoinServerClient{
		JoinReqPayloadChan:   make(chan backend.JoinReqPayload, 100),
		RejoinReqPayloadChan: make(chan backend.RejoinReqPayload, 100),
	}
}

// JoinReq method.
func (c *JoinServerClient) JoinReq(ctx context.Context, pl backend.JoinReqPayload) (backend.JoinAnsPayload, error) {
	c.JoinReqPayloadChan <- pl
	return c.JoinAnsPayload, c.JoinReqError
}

// RejoinReq method.
func (c *JoinServerClient) RejoinReq(ctx context.Context, pl backend.RejoinReqPayload) (backend.RejoinAnsPayload, error) {
	c.RejoinReqPayloadChan <- pl
	return c.RejoinAnsPayload, c.RejoinReqError
}
