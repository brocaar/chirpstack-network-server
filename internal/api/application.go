package api

import (
	"golang.org/x/net/context"

	pb "github.com/brocaar/loraserver/api"
	"github.com/brocaar/loraserver/internal/loraserver"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/loraserver/models"
	"github.com/brocaar/lorawan"
)

// ApplicationAPI exports the Application related functions.
type ApplicationAPI struct {
	ctx loraserver.Context
}

// NewApplicationAPI creates a new ApplicationAPI.
func NewApplicationAPI(ctx loraserver.Context) *ApplicationAPI {
	return &ApplicationAPI{
		ctx: ctx,
	}
}

// Get returns the Application for the given AppEUI.
func (a *ApplicationAPI) Get(ctx context.Context, req *pb.GetApplicationRequest) (*pb.GetApplicationResponse, error) {
	var eui lorawan.EUI64
	if err := eui.UnmarshalText([]byte(req.AppEUI)); err != nil {
		return nil, err
	}

	app, err := storage.GetApplication(a.ctx.DB, eui)
	if err != nil {
		return nil, err
	}
	b, err := app.AppEUI.MarshalText()
	if err != nil {
		return nil, err
	}
	return &pb.GetApplicationResponse{
		AppEUI: string(b),
		Name:   app.Name,
	}, nil
}

// List returns a list of applications (given a limit and offset).
func (a *ApplicationAPI) List(ctx context.Context, req *pb.ListApplicationRequest) (*pb.ListApplicationResponse, error) {
	apps, err := storage.GetApplications(a.ctx.DB, int(req.Limit), int(req.Offset))
	if err != nil {
		return nil, err
	}

	count, err := storage.GetApplicationsCount(a.ctx.DB)
	if err != nil {
		return nil, err
	}

	var resp pb.ListApplicationResponse
	resp.TotalCount = int64(count)
	for _, app := range apps {
		b, err := app.AppEUI.MarshalText()
		if err != nil {
			return nil, err
		}
		resp.Result = append(resp.Result, &pb.GetApplicationResponse{
			AppEUI: string(b),
			Name:   app.Name,
		})
	}

	return &resp, nil
}

// Create creates the given application.
func (a *ApplicationAPI) Create(ctx context.Context, req *pb.CreateApplicationRequest) (*pb.CreateApplicationResponse, error) {
	var eui lorawan.EUI64
	if err := eui.UnmarshalText([]byte(req.AppEUI)); err != nil {
		return nil, err
	}

	if err := storage.CreateApplication(a.ctx.DB, models.Application{AppEUI: eui, Name: req.Name}); err != nil {
		return nil, err
	}

	return &pb.CreateApplicationResponse{}, nil
}

// Update updates the given Application.
func (a *ApplicationAPI) Update(ctx context.Context, req *pb.UpdateApplicationRequest) (*pb.UpdateApplicationResponse, error) {
	var eui lorawan.EUI64
	if err := eui.UnmarshalText([]byte(req.AppEUI)); err != nil {
		return nil, err
	}

	if err := storage.UpdateApplication(a.ctx.DB, models.Application{AppEUI: eui, Name: req.Name}); err != nil {
		return nil, err
	}

	return &pb.UpdateApplicationResponse{}, nil
}

// Delete deletes the application for the given AppEUI.
func (a *ApplicationAPI) Delete(ctx context.Context, req *pb.DeleteApplicationRequest) (*pb.DeleteApplicationResponse, error) {
	var eui lorawan.EUI64
	if err := eui.UnmarshalText([]byte(req.AppEUI)); err != nil {
		return nil, err
	}

	if err := storage.DeleteApplication(a.ctx.DB, eui); err != nil {
		return nil, err
	}

	return &pb.DeleteApplicationResponse{}, nil
}
