package loraserver

import (
	"github.com/brocaar/lorawan"
	"github.com/jmoiron/sqlx"
)

// Application contains the information of an application.
type Application struct {
	AppEUI lorawan.EUI64
	Name   string
}

// CreateApplication creates the given Application
func CreateApplication(db *sqlx.DB, a Application) error {
	_, err := db.Exec("insert into application (app_eui, name) values ($1, $2)",
		a.AppEUI[:],
		a.Name,
	)
	return err
}
