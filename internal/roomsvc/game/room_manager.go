package game

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/cabo/cabo-backend/internal/roomsvc/authclient"
)

// RoomManager tracks every GameRoom currently live on this roomsvc
// instance, keyed by room ID. It is the entry point for creating a new
// room and for looking one up when a second player joins.
type RoomManager struct {
	mu       sync.RWMutex
	rooms    map[string]*GameRoom
	log      *slog.Logger
	notifier authclient.Client
}

// NewRoomManager creates an empty RoomManager. notifier is told about
// every room this instance creates (see CreateRoom) so authsvc can record
// which server owns it (spine AD-3, AD-9); NotifyRoomCreated is
// non-blocking and never fails CreateRoom.
func NewRoomManager(log *slog.Logger, notifier authclient.Client) *RoomManager {
	return &RoomManager{
		rooms:    make(map[string]*GameRoom),
		log:      log,
		notifier: notifier,
	}
}

// CreateRoom creates a new GameRoom via NewGameRoom and registers it so it
// can be found later with GetRoom. If room creation fails (e.g. an invalid
// maxPlayers), nothing is registered and the notifier is not called.
func (m *RoomManager) CreateRoom(firstPlayer *Player, maxPlayers int) (*GameRoom, error) {
	room, err := NewGameRoom(firstPlayer, maxPlayers, m.log)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	m.rooms[room.ID] = room
	roomCount := len(m.rooms)
	m.mu.Unlock()

	m.log.Info("room registered", "room_id", room.ID, "room_count", roomCount)
	m.notifier.NotifyRoomCreated(room.ID)

	return room, nil
}

// GetRoom returns the live room with the given ID, or an error if no such
// room is registered on this instance.
func (m *RoomManager) GetRoom(roomID string) (*GameRoom, error) {
	m.mu.RLock()
	room, ok := m.rooms[roomID]
	m.mu.RUnlock()

	if !ok {
		m.log.Info("room lookup missed", "room_id", roomID)
		return nil, fmt.Errorf("roomsvc: no room registered with id %q", roomID)
	}

	return room, nil
}
