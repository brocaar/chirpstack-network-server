package loraserver

import (
	"errors"

	log "github.com/Sirupsen/logrus"
	"github.com/brocaar/loraserver/models"
	"github.com/brocaar/lorawan"
	"github.com/jmoiron/sqlx"
)

// SQLNodeApplicationsManager implements a SQL version of the NodeApplicationsManager.
type SQLNodeApplicationsManager struct {
	DB *sqlx.DB
}

// NewNodeApplicationsManager creates a new SQLNodeApplicationsManager.
func NewNodeApplicationsManager(db *sqlx.DB) (NodeApplicationsManager, error) {
	return &SQLNodeApplicationsManager{db}, nil
}

// create creates the given Application
func (s *SQLNodeApplicationsManager) create(a models.Application) error {
	_, err := s.DB.Exec("insert into application (app_eui, name) values ($1, $2)",
		a.AppEUI[:],
		a.Name,
	)
	if err == nil {
		log.WithField("app_eui", a.AppEUI).Info("application created")
	}
	return err
}

// get returns the Application for the given AppEUI.
func (s *SQLNodeApplicationsManager) get(appEUI lorawan.EUI64) (models.Application, error) {
	var app models.Application
	return app, s.DB.Get(&app, "select * from application where app_eui = $1", appEUI[:])
}

// getList returns a slice of applications.
func (s *SQLNodeApplicationsManager) getList(limit, offset int) ([]models.Application, error) {
	var apps []models.Application
	return apps, s.DB.Select(&apps, "select * from application order by app_eui limit $1 offset $2", limit, offset)
}

// update updates the given Application.
func (s *SQLNodeApplicationsManager) update(a models.Application) error {
	res, err := s.DB.Exec("update application set name = $1 where app_eui = $2",
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

// delete deletes the Application matching the given AppEUI.
// Note that this will delete all related nodes too!
func (s *SQLNodeApplicationsManager) delete(appEUI lorawan.EUI64) error {
	res, err := s.DB.Exec("delete from application where app_eui = $1",
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
