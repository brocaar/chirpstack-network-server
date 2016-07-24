package storage

import (
	"fmt"

	log "github.com/Sirupsen/logrus"
	"github.com/jmoiron/sqlx"

	"github.com/brocaar/loraserver/models"
)

// CreateChannelList creates the given ChannelList.
func CreateChannelList(db *sqlx.DB, cl *models.ChannelList) error {
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

// UpdateChannelList updates the given ChannelList.
func UpdateChannelList(db *sqlx.DB, cl models.ChannelList) error {
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

// GetChannelList returns the ChannelList for the given id.
func GetChannelList(db *sqlx.DB, id int64) (models.ChannelList, error) {
	var cl models.ChannelList
	err := db.Get(&cl, "select * from channel_list where id = $1", id)
	if err != nil {
		return cl, fmt.Errorf("get channel-list %d error: %s", id, err)
	}
	return cl, nil
}

// GetChannelLists returns a list of ChannelList items.
func GetChannelLists(db *sqlx.DB, limit, offset int) ([]models.ChannelList, error) {
	var channelLists []models.ChannelList
	err := db.Select(&channelLists, "select * from channel_list order by name limit $1 offset $2", limit, offset)
	if err != nil {
		return nil, fmt.Errorf("get channel-list list error: %s", err)
	}
	return channelLists, nil
}

// GetChannelListsCount returns the total number of channel-lists.
func GetChannelListsCount(db *sqlx.DB) (int, error) {
	var count struct {
		Count int
	}
	err := db.Get(&count, "select count(*) as count from channel_list")
	if err != nil {
		return 0, fmt.Errorf("get channel-list count error: %s", err)
	}
	return count.Count, nil
}

// DeleteChannelList deletes the ChannelList matching the given id.
func DeleteChannelList(db *sqlx.DB, id int64) error {
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

// CreateChannel creates the given channel.
func CreateChannel(db *sqlx.DB, c *models.Channel) error {
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

// UpdateChannel updates the given Channel.
func UpdateChannel(db *sqlx.DB, c models.Channel) error {
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

// DeleteChannel deletes the Channel matching the given id.
func DeleteChannel(db *sqlx.DB, id int64) error {
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

// GetChannel returns the Channel matching the given id.
func GetChannel(db *sqlx.DB, id int64) (models.Channel, error) {
	var channel models.Channel
	err := db.Get(&channel, "select * from channel where id = $1", id)
	if err != nil {
		return channel, fmt.Errorf("get channel %d error: %s", id, err)
	}
	return channel, nil
}

// GetChannelsForChannelList returns the Channels for the given ChannelList id.
func GetChannelsForChannelList(db *sqlx.DB, channelListID int64) ([]models.Channel, error) {
	var channels []models.Channel
	err := db.Select(&channels, "select * from channel where channel_list_id = $1 order by channel", channelListID)
	if err != nil {
		return nil, fmt.Errorf("get channels for channel-list %d error: %s", channelListID, err)
	}
	return channels, nil
}
