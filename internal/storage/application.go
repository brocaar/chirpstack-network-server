package storage

import (
	"fmt"

	log "github.com/Sirupsen/logrus"
	"github.com/jmoiron/sqlx"

	"github.com/brocaar/loraserver/models"
	"github.com/brocaar/lorawan"
)

// CreateApplication creates the given Application
func CreateApplication(db *sqlx.DB, a models.Application) error {
	_, err := db.Exec("insert into application (app_eui, name) values ($1, $2)",
		a.AppEUI[:],
		a.Name,
	)
	if err != nil {
		return fmt.Errorf("create application %s error: %s", a.AppEUI, err)
	}
	log.WithField("app_eui", a.AppEUI).Info("application created")
	return nil
}

// GetApplication returns the Application for the given AppEUI.
func GetApplication(db *sqlx.DB, appEUI lorawan.EUI64) (models.Application, error) {
	var app models.Application
	err := db.Get(&app, "select * from application where app_eui = $1", appEUI[:])
	if err != nil {
		return app, fmt.Errorf("get application %s error: %s", appEUI, err)
	}
	return app, nil
}

// GetApplications returns a slice of applications.
func GetApplications(db *sqlx.DB, limit, offset int) ([]models.Application, error) {
	var apps []models.Application
	err := db.Select(&apps, "select * from application order by app_eui limit $1 offset $2", limit, offset)
	if err != nil {
		return apps, fmt.Errorf("get applications error: %s", err)
	}
	return apps, nil
}

// GetApplicationsCount returns the number of applications.
func GetApplicationsCount(db *sqlx.DB) (int, error) {
	var count struct {
		Count int
	}
	err := db.Get(&count, "select count(*) as count from application")
	if err != nil {
		return 0, fmt.Errorf("get applications count error: %s", err)
	}
	return count.Count, nil
}

// UpdateApplication updates the given Application.
func UpdateApplication(db *sqlx.DB, a models.Application) error {
	res, err := db.Exec("update application set name = $1 where app_eui = $2",
		a.Name,
		a.AppEUI[:],
	)
	if err != nil {
		return fmt.Errorf("update application %s error: %s", a.AppEUI, err)
	}
	ra, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if ra == 0 {
		return fmt.Errorf("application %s does not exist", a.AppEUI)
	}
	log.WithField("app_eui", a.AppEUI).Info("application updated")
	return nil
}

// DeleteApplication deletes the Application matching the given AppEUI.
// Note that this will delete all related nodes too!
func DeleteApplication(db *sqlx.DB, appEUI lorawan.EUI64) error {
	res, err := db.Exec("delete from application where app_eui = $1",
		appEUI[:],
	)
	if err != nil {
		return fmt.Errorf("delete application %s error: %s", appEUI, err)
	}
	ra, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if ra == 0 {
		return fmt.Errorf("application %s does not exist", appEUI)
	}

	log.WithField("app_eui", appEUI).Info("application deleted")
	return nil
}
