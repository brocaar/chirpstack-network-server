//go:generate stringer -type=NotificationType

package models

import (
	"time"

	"github.com/brocaar/lorawan"
)

// NotificationType defines the notification.
type NotificationType int

// Available notification types.
const (
	JoinNotificationType = iota
)

// JoinNotification defines the payload sent to the application on
// a JoinNotificationType event.
type JoinNotification struct {
	DevAddr lorawan.DevAddr `json:"devAddr"`
	DevEUI  lorawan.EUI64   `json:"devEUI"`
	Time    time.Time       `json:"time"`
}
