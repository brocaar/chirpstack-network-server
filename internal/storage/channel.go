package storage

import (
	"fmt"

	"github.com/brocaar/loraserver/models"
	"github.com/jmoiron/sqlx"
)

// GetChannelsForChannelList returns the Channels for the given ChannelList id.
func GetChannelsForChannelList(db *sqlx.DB, channelListID int64) ([]models.Channel, error) {
	var channels []models.Channel
	err := db.Select(&channels, "select * from channel where channel_list_id = $1 order by channel", channelListID)
	if err != nil {
		return nil, fmt.Errorf("get channels for channel-list %d error: %s", channelListID, err)
	}
	return channels, nil
}
