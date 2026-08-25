// Package serverpicker chooses a live roomsvc server for a new room, by
// reading the etcd roster roomsvc registers itself in (spine AD-7, written
// by internal/roomsvc/registry.Registrar).
package serverpicker

import (
	"context"
	"errors"
)

// ErrNoServersAvailable is returned by PickServer when no roomsvc server is
// currently registered.
var ErrNoServersAvailable = errors.New("serverpicker: no live roomsvc servers available")

// ServerPicker chooses a live roomsvc server address for a new room.
// Callers depend only on this interface, never on etcd directly, so the
// placement source or policy can change later without changing callers.
type ServerPicker interface {
	PickServer(ctx context.Context) (address string, err error)
}
