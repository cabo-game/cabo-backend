package game

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/cabo/cabo-backend/internal/roomsvc/cards"
	"github.com/cabo/cabo-backend/internal/roomsvc/ws"
)

// Player is one occupant of a GameRoom: their identity, the WebSocket
// connection carrying their messages, and the cards they hold. Hand is
// nil until GameRoom.StartGame deals cards.
//
// ViewedInitialCards tracks whether this player has already used their
// one-time initial peek (see GameRoom.ViewInitialCards) — Cabo players do
// not know their own hand by default, and the initial peek can only
// happen once.
//
// DrawnCard is the card this player currently holds from the draw pile but
// has not yet resolved (swapped into Hand or discarded — neither is built
// yet). It is nil when nothing is pending. A draw does not change len(Hand)
// — Cabo only changes hand size when a drawn card is later resolved, never
// on the draw itself.
type Player struct {
	ID                 string
	Conn               *ws.Connection
	Hand               []cards.Card
	ViewedInitialCards bool
	DrawnCard          *cards.Card
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
