package eventhub

import "errors"

type (
	Event struct {
		Type   EventType `json:"type"`
		FileID string    `json:"file_id"`
		From   string    `json:"from"`
		To     string    `json:"to"`
	}

	EventType string
)

const (
	EventTypeCreate = "create"
	EventTypeModify = "modify"
	EventTypeRename = "rename"
	EventTypeDelete = "delete"
)

var (
	// ErrEventHubClosed is returned when operations are attempted on a closed EventHub.
	ErrEventHubClosed = errors.New("event hub is closed")
)
