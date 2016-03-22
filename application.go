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

// createApplication creates the given Application
func createApplication(db *sqlx.DB, a Application) error {
	_, err := db.Exec("insert into application (app_eui, name) values ($1, $2)",
		a.AppEUI[:],
		a.Name,
	)
	if err == nil {
		log.WithField("app_eui", a.AppEUI).Info("application created")
	}
	return err
}

// getApplication returns the Application for the given AppEUI.
func getApplication(db *sqlx.DB, appEUI lorawan.EUI64) (Application, error) {
	var app Application
	return app, db.Get(&app, "select * from application where app_eui = $1", appEUI[:])
}

// getApplications returns a slice of applications.
func getApplications(db *sqlx.DB, limit, offset int) ([]Application, error) {
	var apps []Application
	return apps, db.Select(&apps, "select * from application order by app_eui limit $1 offset $2", limit, offset)
}

// updateApplication updates the given Application.
func updateApplication(db *sqlx.DB, a Application) error {
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

// deleteApplication deletes the Application matching the given AppEUI.
// Note that this will delete all related nodes too!
func deleteApplication(db *sqlx.DB, appEUI lorawan.EUI64) error {
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

// ApplicationAPI exports the Application related functions.
type ApplicationAPI struct {
	ctx Context
}

// NewApplicationAPI creates a new ApplicationAPI.
func NewApplicationAPI(ctx Context) *ApplicationAPI {
	return &ApplicationAPI{
		ctx: ctx,
	}
}

// Get returns the Application for the given AppEUI.
func (a *ApplicationAPI) Get(appEUI lorawan.EUI64, app *Application) error {
	var err error
	*app, err = getApplication(a.ctx.DB, appEUI)
	return err
}

// GetList returns a list of applications (given a limit and offset).
func (a *ApplicationAPI) GetList(req GetListRequest, apps *[]Application) error {
	var err error
	*apps, err = getApplications(a.ctx.DB, req.Limit, req.Offset)
	return err
}

// Create creates the given application.
func (a *ApplicationAPI) Create(app Application, appEUI *lorawan.EUI64) error {
	if err := createApplication(a.ctx.DB, app); err != nil {
		return err
	}
	*appEUI = app.AppEUI
	return nil
}

// Update updates the given Application.
func (a *ApplicationAPI) Update(app Application, appEUI *lorawan.EUI64) error {
	if err := updateApplication(a.ctx.DB, app); err != nil {
		return err
	}
	*appEUI = app.AppEUI
	return nil
}

// Delete deletes the application for the given AppEUI.
func (a *ApplicationAPI) Delete(appEUI lorawan.EUI64, deletedAppEUI *lorawan.EUI64) error {
	if err := deleteApplication(a.ctx.DB, appEUI); err != nil {
		return err
	}
	*deletedAppEUI = appEUI
	return nil
}
