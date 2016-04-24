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
	JoinNotification = iota
)

// JoinNotificationPayload defines the payload sent to the application on
// a JoinNotification event.
type JoinNotificationPayload struct {
	DevEUI lorawan.EUI64 `json:"devEUI"`
	Time   time.Time     `json:"time"`
}
