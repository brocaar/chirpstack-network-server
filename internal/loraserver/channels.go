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

// createChannel creates the given channel.
func createChannel(db *sqlx.DB, c *models.Channel) error {
	err := db.Get(&c.ID, "insert into channel (channel_set_id, channel, frequency) values ($1, $2, $3) returning id",
		c.ChannelSetID,
		c.Channel,
		c.Frequency,
	)
	if err != nil {
		return fmt.Errorf("create channel %d for channel-set %d error: %s", c.Channel, c.ChannelSetID, err)
	}
	log.WithFields(log.Fields{
		"channel_set_id": c.ChannelSetID,
		"channel":        c.Channel,
		"id":             c.ID,
	}).Info("channel created")
	return nil
}

// updateChannel updates the given Channel.
func updateChannel(db *sqlx.DB, c models.Channel) error {
	res, err := db.Exec("update channel set channel_set_id = $1, channel = $2, frequency = $3 where id = $4",
		c.ChannelSetID,
		c.Channel,
		c.Frequency,
		c.ID,
	)
	if err != nil {
		return fmt.Errorf("update channel %d error: %s", c.ID, err)
	}
	ra, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if ra == 0 {
		return fmt.Errorf("channel %d does not exist", c.ID)
	}
	log.WithField("id", c.ID).Info("channel updated")
	return nil
}

// deleteChannel deletes the Channel matching the given id.
func deleteChannel(db *sqlx.DB, id int64) error {
	res, err := db.Exec("delete from channel where id = $1",
		id,
	)
	if err != nil {
		return fmt.Errorf("delete channel %d error: %s", id, err)
	}
	ra, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if ra == 0 {
		return fmt.Errorf("channel %d does not exist", id)
	}
	log.WithField("id", id).Info("channel deleted")
	return nil
}

// getChannel returns the Channel matching the given id.
func getChannel(db *sqlx.DB, id int64) (models.Channel, error) {
	var channel models.Channel
	err := db.Get(&channel, "select * from channel where id = $1", id)
	if err != nil {
		return channel, fmt.Errorf("get channel %d error: %s", id, err)
	}
	return channel, nil
}

// getChannels returns the Channels for the given ChannelSet id.
func getChannelsForChannelSet(db *sqlx.DB, channelSetID int64) ([]models.Channel, error) {
	var channels []models.Channel
	err := db.Select(&channels, "select * from channel where channel_set_id = $1 order by channel", channelSetID)
	if err != nil {
		return nil, fmt.Errorf("get channels for channel-set %d error: %s", channelSetID, err)
	}
	return channels, nil
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

// ChannelAPI struct exports the channel related functions.
type ChannelAPI struct {
	ctx Context
}

// NewChannelAPI creates a new ChannelAPI.
func NewChannelAPI(ctx Context) *ChannelAPI {
	return &ChannelAPI{
		ctx: ctx,
	}
}

// Create creates the given Channel.
func (a *ChannelAPI) Create(c models.Channel, id *int64) error {
	if err := createChannel(a.ctx.DB, &c); err != nil {
		return err
	}
	*id = c.ID
	return nil
}

// Update updates the given Channel.
func (a *ChannelAPI) Update(c models.Channel, id *int64) error {
	if err := updateChannel(a.ctx.DB, c); err != nil {
		return err
	}
	*id = c.ID
	return nil
}

// Delete deletes the Channel matching the given id.
func (a *ChannelAPI) Delete(id int64, deletedID *int64) error {
	if err := deleteChannel(a.ctx.DB, id); err != nil {
		return err
	}
	*deletedID = id
	return nil
}

// GetForChannelSet returns the channels for the given ChannelSet id.
func (a *ChannelAPI) GetForChannelSet(id int64, channels *[]models.Channel) error {
	var err error
	*channels, err = getChannelsForChannelSet(a.ctx.DB, id)
	return err
}

// Get returns the Channel matching the given id.
func (a *ChannelAPI) Get(id int64, channel *models.Channel) error {
	var err error
	*channel, err = getChannel(a.ctx.DB, id)
	return err
}
