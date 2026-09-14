package game

import "github.com/cabo/cabo-backend/internal/roomsvc/cards"

// ActionView is the narrow slice of GameRoom that gameplay action handlers
// (package gameplay/actions) are allowed to see and mutate. GameRoom's full
// API — in particular Players []*Player, which exposes each Player's
// *ws.Connection — is deliberately not handed to actions: a *GameRoom
// reference would let an action reach room.Players[i].Conn.Write(...)
// directly, bypassing the redaction and delivery path the gameplay
// dispatcher owns (see internal/roomsvc/GAMEPLAY_LLD_DRAFT.md, "package
// boundary is the security boundary").
//
// *GameRoom implements ActionView. Callers outside this package should
// depend on ActionView, never on *GameRoom, when constructing an
// ActionContext for an action handler.
type ActionView interface {
	// CurrentPlayerID returns the ID of the player whose turn it is.
	CurrentPlayerID() string

	// AdvanceTurn moves the turn to the next player in seat order.
	AdvanceTurn()

	// HandOf returns a copy of playerID's current hand.
	HandOf(playerID string) ([]cards.Card, error)

	// DrawTopCard pops the draw pile's top card into playerID's pending
	// DrawnCard. See GameRoom.DrawTopCard for the full contract.
	DrawTopCard(playerID string) (cards.Card, error)

	// MarkInitialCardsViewed records that playerID has used their
	// one-time initial peek. See GameRoom.MarkInitialCardsViewed.
	MarkInitialCardsViewed(playerID string) error

	// SeatedPlayerIDs returns the IDs of every currently seated player,
	// in seat order — exposed as IDs only, never *Player, so an action
	// still has no path to a live *ws.Connection.
	SeatedPlayerIDs() []string
}

var _ ActionView = (*GameRoom)(nil)
