package jsclient

import (
	"github.com/Frankz/lorawan"
)

// Pool defines the join-server client pool.
type Pool interface {
	Get(joinEUI lorawan.EUI64) (Client, error)
}

type pool struct {
	defaultClient Client
}

// NewPool creates a new Pool.
func NewPool(defaultClient Client) Pool {
	return &pool{
		defaultClient: defaultClient,
	}
}

// Get returns the join-server client for the given joinEUI.
func (p *pool) Get(joinEUI lorawan.EUI64) (Client, error) {
	return p.defaultClient, nil
}
