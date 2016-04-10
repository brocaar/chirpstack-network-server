package models

// GetListRequest represents the request for getting a list of objects.
type GetListRequest struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}
