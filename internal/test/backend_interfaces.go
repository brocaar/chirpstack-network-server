package test

import (
	"github.com/brocaar/lorawan/backend"
	context "golang.org/x/net/context"
)

// BackendClient is a LoRaWAN Backend Interfaces client for testing.
type BackendClient struct {
	SenderID            string
	ReceiverID          string
	Async               bool
	RandomTransactionID uint32

	PRStartReqChan  chan backend.PRStartReqPayload
	PRStopReqChan   chan backend.PRStopReqPayload
	XmitDataReqChan chan backend.XmitDataReqPayload
	ProfileReqChan  chan backend.ProfileReqPayload
	HomeNSReqChan   chan backend.HomeNSReqPayload

	PRStartAns  backend.PRStartAnsPayload
	PRStopAns   backend.PRStopAnsPayload
	XmitDataAns backend.XmitDataAnsPayload
	ProfileAns  backend.ProfileAnsPayload
	HomeNSAns   backend.HomeNSAnsPayload
}

// NewBackendClient creates a new BackendClient.
func NewBackendClient() *BackendClient {
	return &BackendClient{
		PRStartReqChan:  make(chan backend.PRStartReqPayload, 10),
		PRStopReqChan:   make(chan backend.PRStopReqPayload, 10),
		XmitDataReqChan: make(chan backend.XmitDataReqPayload, 10),

		ProfileReqChan: make(chan backend.ProfileReqPayload, 10),
		HomeNSReqChan:  make(chan backend.HomeNSReqPayload, 10),
	}
}

// GetSenderID returns the SenderID.
func (c *BackendClient) GetSenderID() string {
	return c.SenderID
}

// GetReceiverID returns the ReceiverID.
func (c *BackendClient) GetReceiverID() string {
	return c.ReceiverID
}

// IsAsync returns a bool indicating if the client is async.
func (c *BackendClient) IsAsync() bool {
	return c.Async
}

// GetRandomTransactionID returns a random transaction id.
func (c *BackendClient) GetRandomTransactionID() uint32 {
	return c.RandomTransactionID
}

// PRStartReq method.
func (c *BackendClient) PRStartReq(ctx context.Context, req backend.PRStartReqPayload) (backend.PRStartAnsPayload, error) {
	c.PRStartReqChan <- req
	return c.PRStartAns, nil
}

// PRStopReq method.
func (c *BackendClient) PRStopReq(ctx context.Context, req backend.PRStopReqPayload) (backend.PRStopAnsPayload, error) {
	c.PRStopReqChan <- req
	return c.PRStopAns, nil
}

// XmitDataReq method.
func (c *BackendClient) XmitDataReq(ctx context.Context, req backend.XmitDataReqPayload) (backend.XmitDataAnsPayload, error) {
	c.XmitDataReqChan <- req
	return c.XmitDataAns, nil
}

// ProfileReq method.
func (c *BackendClient) ProfileReq(ctx context.Context, req backend.ProfileReqPayload) (backend.ProfileAnsPayload, error) {
	c.ProfileReqChan <- req
	return c.ProfileAns, nil
}

// HomeNSReq method.
func (c *BackendClient) HomeNSReq(ctx context.Context, req backend.HomeNSReqPayload) (backend.HomeNSAnsPayload, error) {
	c.HomeNSReqChan <- req
	return c.HomeNSAns, nil
}

// SendAnswer sends the async answer.
func (c *BackendClient) SendAnswer(context.Context, backend.Answer) error {
	panic("not implemented")
}

// HandleAnswer handles an async answer.
func (c *BackendClient) HandleAnswer(context.Context, backend.Answer) error {
	panic("not implemented")
}
