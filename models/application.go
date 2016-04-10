// Package models contains all the exported lorawan models.
package models

import "github.com/brocaar/lorawan"

// Application contains the information of an application.
type Application struct {
	AppEUI lorawan.EUI64 `db:"app_eui" json:"appEUI"`
	Name   string        `db:"name" json:"name"`
}
