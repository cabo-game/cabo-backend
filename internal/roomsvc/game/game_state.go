package game

import "github.com/cabo/cabo-backend/internal/roomsvc/cards"

// GameState is the authoritative, shared state of one game round, as
// opposed to per-player state (see Player.Hand). Its shape is still
// mostly a placeholder: deck size is decided (see cabo-bmad/docs/cabo.md,
// "Deck size: standard 52-card deck"), but card values, special powers,
// and turn/round state are deferred to the game-logic spec and
// deliberately not added here yet.
type GameState struct {
	// RemainingCards is the draw pile: whatever is left of the deck after
	// GameRoom.StartGame deals opening hands to every player. Empty until
	// StartGame runs.
	RemainingCards []cards.Card
}
