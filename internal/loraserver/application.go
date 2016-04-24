package loraserver

import (
	"github.com/brocaar/loraserver/models"
	"github.com/brocaar/lorawan"
)

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
func (a *ApplicationAPI) Get(appEUI lorawan.EUI64, app *models.Application) error {
	var err error
	*app, err = a.ctx.NodeAppManager.get(appEUI)
	return err
}

// GetList returns a list of applications (given a limit and offset).
func (a *ApplicationAPI) GetList(req models.GetListRequest, apps *[]models.Application) error {
	var err error
	*apps, err = a.ctx.NodeAppManager.getList(req.Limit, req.Offset)
	return err
}

// Create creates the given application.
func (a *ApplicationAPI) Create(app models.Application, appEUI *lorawan.EUI64) error {
	if err := a.ctx.NodeAppManager.create(app); err != nil {
		return err
	}
	*appEUI = app.AppEUI
	return nil
}

// Update updates the given Application.
func (a *ApplicationAPI) Update(app models.Application, appEUI *lorawan.EUI64) error {
	if err := a.ctx.NodeAppManager.update(app); err != nil {
		return err
	}
	*appEUI = app.AppEUI
	return nil
}

// Delete deletes the application for the given AppEUI.
func (a *ApplicationAPI) Delete(appEUI lorawan.EUI64, deletedAppEUI *lorawan.EUI64) error {
	if err := a.ctx.NodeAppManager.delete(appEUI); err != nil {
		return err
	}
	*deletedAppEUI = appEUI
	return nil
}
