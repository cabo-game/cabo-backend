package game

import "github.com/cabo/cabo-backend/internal/roomsvc/cards"

// GameState is the authoritative, shared state of one game round, as
// opposed to per-player state (see Player.Hand). Its shape is still
// mostly a placeholder: deck size is decided (see cabo-bmad/docs/cabo.md,
// "Deck size: standard 52-card deck"), but card values, special powers,
// and full round state are deferred to the game-logic spec and
// deliberately not added here yet.
type GameState struct {
	// RemainingCards is the draw pile: whatever is left of the deck after
	// GameRoom.StartGame deals opening hands to every player. Empty until
	// StartGame runs.
	RemainingCards []cards.Card

	// TurnOrder is the seat order turns cycle through, fixed at StartGame
	// time (the order players were seated in). Empty until StartGame runs.
	TurnOrder []string

	// CurrentTurnIdx indexes into TurnOrder for whose turn it currently
	// is. Only meaningful once TurnOrder is populated.
	CurrentTurnIdx int
}

// CurrentPlayerID returns the ID of the player whose turn it currently is.
// Returns "" if the game hasn't started (TurnOrder is empty).
func (s *GameState) CurrentPlayerID() string {
	if len(s.TurnOrder) == 0 {
		return ""
	}
	return s.TurnOrder[s.CurrentTurnIdx]
}

// AdvanceTurn moves to the next player in TurnOrder, wrapping back to the
// start after the last player.
func (s *GameState) AdvanceTurn() {
	if len(s.TurnOrder) == 0 {
		return
	}
	s.CurrentTurnIdx = (s.CurrentTurnIdx + 1) % len(s.TurnOrder)
}
