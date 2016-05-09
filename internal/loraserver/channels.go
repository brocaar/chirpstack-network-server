package loraserver

import (
	"fmt"

	log "github.com/Sirupsen/logrus"
	"github.com/jmoiron/sqlx"

	"github.com/brocaar/loraserver/models"
)

// createChannelList creates the given ChannelList.
func createChannelList(db *sqlx.DB, cl *models.ChannelList) error {
	err := db.Get(&cl.ID, "insert into channel_list (name) values ($1) returning id",
		cl.Name,
	)
	if err != nil {
		return fmt.Errorf("create channel-list '%s' error: %s", cl.Name, err)
	}
	log.WithFields(log.Fields{
		"id":   cl.ID,
		"name": cl.Name,
	}).Info("channel-list created")
	return nil
}

// updateChannelList updates the given ChannelList.
func updateChannelList(db *sqlx.DB, cl models.ChannelList) error {
	res, err := db.Exec("update channel_list set name = $1 where id = $2",
		cl.Name,
		cl.ID,
	)
	if err != nil {
		return fmt.Errorf("update channel-list %d error: %s", cl.ID, err)
	}
	ra, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if ra == 0 {
		return fmt.Errorf("channel-list %d does not exist", cl.ID)
	}
	log.WithField("id", cl.ID).Info("channel-list updated")
	return nil
}

// getChannelList returns the ChannelList for the given id.
func getChannelList(db *sqlx.DB, id int64) (models.ChannelList, error) {
	var cl models.ChannelList
	err := db.Get(&cl, "select * from channel_list where id = $1", id)
	if err != nil {
		return cl, fmt.Errorf("get channel-list %d error: %s", id, err)
	}
	return cl, nil
}

// getChannelLists returns a list of ChannelList items.
func getChannelLists(db *sqlx.DB, limit, offset int) ([]models.ChannelList, error) {
	var channelLists []models.ChannelList
	err := db.Select(&channelLists, "select * from channel_list order by name limit $1 offset $2", limit, offset)
	if err != nil {
		return nil, fmt.Errorf("get channel-list list error: %s", err)
	}
	return channelLists, nil
}

// deleteChannelList deletes the ChannelList matching the given id.
func deleteChannelList(db *sqlx.DB, id int64) error {
	res, err := db.Exec("delete from channel_list where id = $1",
		id,
	)
	if err != nil {
		return fmt.Errorf("delete channel-list %d error: %s", id, err)
	}
	ra, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if ra == 0 {
		return fmt.Errorf("channel-list %d does not exist", id)
	}
	log.WithField("id", id).Info("channel-list deleted")
	return nil
}

// createChannel creates the given channel.
func createChannel(db *sqlx.DB, c *models.Channel) error {
	err := db.Get(&c.ID, "insert into channel (channel_list_id, channel, frequency) values ($1, $2, $3) returning id",
		c.ChannelListID,
		c.Channel,
		c.Frequency,
	)
	if err != nil {
		return fmt.Errorf("create channel %d for channel-list %d error: %s", c.Channel, c.ChannelListID, err)
	}
	log.WithFields(log.Fields{
		"channel_list_id": c.ChannelListID,
		"channel":         c.Channel,
		"id":              c.ID,
	}).Info("channel created")
	return nil
}

// updateChannel updates the given Channel.
func updateChannel(db *sqlx.DB, c models.Channel) error {
	res, err := db.Exec("update channel set channel_list_id = $1, channel = $2, frequency = $3 where id = $4",
		c.ChannelListID,
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

// getChannelsForChannelList returns the Channels for the given ChannelList id.
func getChannelsForChannelList(db *sqlx.DB, channelListID int64) ([]models.Channel, error) {
	var channels []models.Channel
	err := db.Select(&channels, "select * from channel where channel_list_id = $1 order by channel", channelListID)
	if err != nil {
		return nil, fmt.Errorf("get channels for channel-list %d error: %s", channelListID, err)
	}
	return channels, nil
}

// ChannelListAPI struct exports the channel-list related functions.
type ChannelListAPI struct {
	ctx Context
}

// NewChannelListAPI creates a new ChannelAPI.
func NewChannelListAPI(ctx Context) *ChannelListAPI {
	return &ChannelListAPI{
		ctx: ctx,
	}
}

// Create creates the given ChannelList.
func (a *ChannelListAPI) Create(cl models.ChannelList, id *int64) error {
	if err := createChannelList(a.ctx.DB, &cl); err != nil {
		return err
	}
	*id = cl.ID
	return nil
}

// Update updates the given ChannelList.
func (a *ChannelListAPI) Update(cl models.ChannelList, id *int64) error {
	if err := updateChannelList(a.ctx.DB, cl); err != nil {
		return err
	}
	*id = cl.ID
	return nil
}

// Get returns the ChannelList matching the given id.
func (a *ChannelListAPI) Get(id int64, cl *models.ChannelList) error {
	var err error
	*cl, err = getChannelList(a.ctx.DB, id)
	return err
}

// GetList returns a list of ChannelList items.
func (a *ChannelListAPI) GetList(req models.GetListRequest, sets *[]models.ChannelList) error {
	var err error
	*sets, err = getChannelLists(a.ctx.DB, req.Limit, req.Offset)
	return err
}

// Delete deletes the ChannelList matching the given id.
func (a *ChannelListAPI) Delete(id int64, deletedID *int64) error {
	if err := deleteChannelList(a.ctx.DB, id); err != nil {
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

// GetForChannelList returns the channels for the given ChannelList id.
func (a *ChannelAPI) GetForChannelList(id int64, channels *[]models.Channel) error {
	var err error
	*channels, err = getChannelsForChannelList(a.ctx.DB, id)
	return err
}

// Get returns the Channel matching the given id.
func (a *ChannelAPI) Get(id int64, channel *models.Channel) error {
	var err error
	*channel, err = getChannel(a.ctx.DB, id)
	return err
}
