package models

import "github.com/brocaar/lorawan"

// GetListRequest represents the request for getting a list of objects.
type GetListRequest struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

// GetListForAppEUIRequest represents the request for getting a list of
// objects for the given AppEUI.
type GetListForAppEUIRequest struct {
	AppEUI lorawan.EUI64 `json:"appEUI"`
	Limit  int           `json:"limit"`
	Offset int           `json:"offset"`
}
