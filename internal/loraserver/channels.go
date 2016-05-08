package loraserver

import (
	"fmt"

	log "github.com/Sirupsen/logrus"
	"github.com/jmoiron/sqlx"

	"github.com/brocaar/loraserver/models"
)

// createChannelSet creates the given ChannelSet.
func createChannelSet(db *sqlx.DB, cs *models.ChannelSet) error {
	err := db.Get(&cs.ID, "insert into channel_set (name) values ($1) returning id",
		cs.Name,
	)
	if err != nil {
		return fmt.Errorf("create channel-set '%s' error: %s", cs.Name, err)
	}
	log.WithFields(log.Fields{
		"id":   cs.ID,
		"name": cs.Name,
	}).Info("channel-set created")
	return nil
}

// updateChannelSet updates the given ChannelSet.
func updateChannelSet(db *sqlx.DB, cs models.ChannelSet) error {
	res, err := db.Exec("update channel_set set name = $1 where id = $2",
		cs.Name,
		cs.ID,
	)
	if err != nil {
		return fmt.Errorf("update channel-set %d error: %s", cs.ID, err)
	}
	ra, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if ra == 0 {
		return fmt.Errorf("channel-set %d does not exist", cs.ID)
	}
	log.WithField("id", cs.ID).Info("channel-set updated")
	return nil
}

// getChannelSet returns the ChannelSet for the given id.
func getChannelSet(db *sqlx.DB, id int64) (models.ChannelSet, error) {
	var cs models.ChannelSet
	err := db.Get(&cs, "select * from channel_set where id = $1", id)
	if err != nil {
		return cs, fmt.Errorf("get channel-set %d error: %s", id, err)
	}
	return cs, nil
}

// getChannelSets returns a list of ChannelSet items.
func getChannelSets(db *sqlx.DB, limit, offset int) ([]models.ChannelSet, error) {
	var channelSets []models.ChannelSet
	err := db.Select(&channelSets, "select * from channel_set order by name limit $1 offset $2", limit, offset)
	if err != nil {
		return nil, fmt.Errorf("get channel-sets error: %s", err)
	}
	return channelSets, nil
}

// deleteChannelSet deletes the ChannelSet matching the given id.
func deleteChannelSet(db *sqlx.DB, id int64) error {
	res, err := db.Exec("delete from channel_set where id = $1",
		id,
	)
	if err != nil {
		return fmt.Errorf("delete channel-set %d error: %s", id, err)
	}
	ra, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if ra == 0 {
		return fmt.Errorf("channel-set %d does not exist", id)
	}
	log.WithField("id", id).Info("channel-set deleted")
	return nil
}

// ChannelSetAPI struct exports the channel-set related functions.
type ChannelSetAPI struct {
	ctx Context
}

// NewChannelSetAPI creates a new ChannelAPI.
func NewChannelSetAPI(ctx Context) *ChannelSetAPI {
	return &ChannelSetAPI{
		ctx: ctx,
	}
}

// Create creates the given ChannelSet.
func (a *ChannelSetAPI) Create(cs models.ChannelSet, id *int64) error {
	if err := createChannelSet(a.ctx.DB, &cs); err != nil {
		return err
	}
	*id = cs.ID
	return nil
}

// Update updates the given ChannelSet.
func (a *ChannelSetAPI) Update(cs models.ChannelSet, id *int64) error {
	if err := updateChannelSet(a.ctx.DB, cs); err != nil {
		return err
	}
	*id = cs.ID
	return nil
}

// Get returns the ChannelSet matching the given id.
func (a *ChannelSetAPI) Get(id int64, cs *models.ChannelSet) error {
	var err error
	*cs, err = getChannelSet(a.ctx.DB, id)
	return err
}

// GetList returns a list of ChannelSet items.
func (a *ChannelSetAPI) GetList(req models.GetListRequest, sets *[]models.ChannelSet) error {
	var err error
	*sets, err = getChannelSets(a.ctx.DB, req.Limit, req.Offset)
	return err
}

// Delete deletes the ChannelSet matching the given id.
func (a *ChannelSetAPI) Delete(id int64, deletedID *int64) error {
	if err := deleteChannelSet(a.ctx.DB, id); err != nil {
		return err
	}
	*deletedID = id
	return nil
}
