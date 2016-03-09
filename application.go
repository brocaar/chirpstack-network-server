package loraserver

import (
	"errors"

	"github.com/brocaar/lorawan"
	"github.com/jmoiron/sqlx"

	log "github.com/Sirupsen/logrus"
)

// Application contains the information of an application.
type Application struct {
	AppEUI lorawan.EUI64 `db:"app_eui" json:"appEUI"`
	Name   string        `db:"name" json:"name"`
}

// CreateApplication creates the given Application
func CreateApplication(db *sqlx.DB, a Application) error {
	_, err := db.Exec("insert into application (app_eui, name) values ($1, $2)",
		a.AppEUI[:],
		a.Name,
	)
	if err == nil {
		log.WithField("app_eui", a.AppEUI).Info("application created")
	}
	return err
}

// GetApplication returns the Application for the given AppEUI.
func GetApplication(db *sqlx.DB, appEUI lorawan.EUI64) (Application, error) {
	var app Application
	return app, db.Get(&app, "select * from application where app_eui = $1", appEUI[:])
}

// GetApplications returns a slice of applications.
func GetApplications(db *sqlx.DB, limit, offset int) ([]Application, error) {
	var apps []Application
	return apps, db.Select(&apps, "select * from application order by app_eui limit $1 offset $2", limit, offset)
}

// UpdateApplication updates the given Application.
func UpdateApplication(db *sqlx.DB, a Application) error {
	res, err := db.Exec("update application set name = $1 where app_eui = $2",
		a.Name,
		a.AppEUI[:],
	)
	if err != nil {
		return err
	}
	ra, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if ra == 0 {
		return errors.New("AppEUI did not match any rows")
	}
	log.WithField("app_eui", a.AppEUI).Info("application updated")
	return nil
}

// DeleteApplication deletes the Application matching the given AppEUI.
func DeleteApplication(db *sqlx.DB, appEUI lorawan.EUI64) error {
	res, err := db.Exec("delete from application where app_eui = $1",
		appEUI[:],
	)
	if err != nil {
		return err
	}
	ra, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if ra == 0 {
		return errors.New("AppEUI did not match any rows")
	}

	log.WithField("app_eui", appEUI).Info("application deleted")
	return nil
}
