package game

import "github.com/cabo/cabo-backend/internal/roomsvc/ws"

// Player is one occupant of a GameRoom: their identity plus the WebSocket
// connection carrying their messages.
type Player struct {
	ID   string
	Conn *ws.Connection
}
