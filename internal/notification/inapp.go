package notification

import (
	"context"

	"argus/internal/db"
)

// NotificationStore is the narrow slice of *db.DB an InAppNotificationStore
// needs — same "narrow interface" convention internal/service's
// BrokerSyncStore etc. follow.
type NotificationStore interface {
	SaveNotification(db.Notification) error
}

// InAppNotificationStore persists every Event into the notifications table
// (migration 24) so a future Web/App surface can list past alerts.
type InAppNotificationStore struct {
	store NotificationStore
}

func NewInAppNotificationStore(store NotificationStore) *InAppNotificationStore {
	return &InAppNotificationStore{store: store}
}

func (n *InAppNotificationStore) Send(ctx context.Context, e Event) error {
	return n.store.SaveNotification(db.Notification{Type: e.Type, Text: e.Text, Level: string(e.Level)})
}
