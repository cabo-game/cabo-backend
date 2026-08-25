// Package roomdirectory maps a room ID to the address of the roomsvc
// server that owns it (spine AD-3). Callers depend only on the
// RoomDirectory interface, never on a specific backing store, so the store
// can be swapped (e.g. Redis today, a SQL/NoSQL store later) by writing a
// new implementation and changing one constructor call in main.go.
package roomdirectory

import (
	"context"
	"errors"
)

// ErrRoomNotFound is returned by LookupRoom when no server is registered
// for the given room ID.
var ErrRoomNotFound = errors.New("roomdirectory: room not found")

// RoomDirectory maps a room ID to the address of the roomsvc server that
// owns it.
type RoomDirectory interface {
	// RegisterRoom records that roomID's server is serverAddress.
	RegisterRoom(ctx context.Context, roomID, serverAddress string) error

	// LookupRoom returns the server address registered for roomID, or
	// ErrRoomNotFound if none is registered.
	LookupRoom(ctx context.Context, roomID string) (serverAddress string, err error)
}
