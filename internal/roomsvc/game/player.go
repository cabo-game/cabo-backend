package game

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/cabo/cabo-backend/internal/roomsvc/ws"
)

// Player is one occupant of a GameRoom: their identity plus the WebSocket
// connection carrying their messages.
type Player struct {
	ID   string
	Conn *ws.Connection
}

// playerIDBytes is the number of random bytes used to generate a player
// ID — 16 bytes (128 bits) makes collisions astronomically unlikely
// without needing a uniqueness check against other live players.
const playerIDBytes = 16

// NewPlayer creates a Player for conn with a freshly generated, random ID.
// This ID is not authentication — real player identity is a separate,
// undecided design effort (see CLAUDE.md's Current State). It only needs
// to be unique enough to distinguish players within this instance's rooms.
func NewPlayer(conn *ws.Connection) *Player {
	return &Player{ID: generatePlayerID(), Conn: conn}
}

func generatePlayerID() string {
	b := make([]byte, playerIDBytes)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("roomsvc: failed to read random bytes for player id: %v", err))
	}
	return hex.EncodeToString(b)
}
