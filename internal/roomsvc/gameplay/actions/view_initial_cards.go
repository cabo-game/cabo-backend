package actions

import (
	"encoding/json"

	"github.com/cabo/cabo-backend/internal/roomsvc/game"
)

// ViewInitialCardsAction lets a player see the first game.InitialPeekCount
// cards of their own hand, once, at the start of a round. Not turn-gated
// — every seated player gets this one-time peek regardless of whose turn
// it is. Only the viewer is told anything; no other player is notified
// (a private peek at your own cards is not observable by anyone else).
//
// Exported so package gameplay's action registry can derive its key from
// ViewInitialCardsAction{}.Name() instead of a duplicated string literal.
type ViewInitialCardsAction struct{}

func (ViewInitialCardsAction) Name() string { return "view_initial_cards" }

func (ViewInitialCardsAction) RequiresCurrentTurn() bool { return false }

func (ViewInitialCardsAction) Handle(ctx ActionContext) (Effect, error) {
	hand, err := ctx.Room.HandOf(ctx.ActingPlayerID)
	if err != nil {
		return Effect{}, err
	}

	peekCount := game.InitialPeekCount
	if peekCount > len(hand) {
		peekCount = len(hand)
	}

	if err := ctx.Room.MarkInitialCardsViewed(ctx.ActingPlayerID); err != nil {
		return Effect{}, err
	}

	payload, err := json.Marshal(struct {
		Cards any `json:"cards"`
	}{Cards: hand[:peekCount]})
	if err != nil {
		return Effect{}, err
	}

	return Effect{
		Recipients: []Recipient{
			{
				PlayerID: ctx.ActingPlayerID,
				View:     PlayerView{Event: "initial_cards", Payload: payload},
			},
		},
	}, nil
}
