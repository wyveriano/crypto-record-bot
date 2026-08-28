package ports

import "context"

// Notifier defines the outgoing port for sending notifications to users without coupling to a specific transport.
type Notifier interface {
	Notify(ctx context.Context, chatID int64, message string) error
}
