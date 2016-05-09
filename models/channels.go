package models

// ChannelList represents a list of channels.
type ChannelList struct {
	ID   int64  `db:"id" json:"id"`
	Name string `db:"name" json:"name"`
}

// Channel represents a single channel.
type Channel struct {
	ID            int64 `db:"id" json:"id"`
	ChannelListID int64 `db:"channel_list_id" json:"channelListID"`
	Channel       int   `db:"channel" json:"channel"`
	Frequency     int   `db:"frequency" json:"frequency"`
}
