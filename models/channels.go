package models

// ChannelSet represents a set of channels.
type ChannelSet struct {
	ID   int64  `db:"id" json:"id"`
	Name string `db:"name" json:"name"`
}

// Channel represents a single channel.
type Channel struct {
	ID           int64 `db:"id" json:"id"`
	ChannelSetID int64 `db:"channel_set_id" json:"channelSetID"`
	Channel      int   `db:"channel" json:"channel"`
	Frequency    int   `db:"frequency" json:"frequency"`
}
