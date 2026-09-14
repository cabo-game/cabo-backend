package actions

import (
	"fmt"

	"github.com/cabo/cabo-backend/internal/roomsvc/cards"
)

// fakeActionView is a minimal, in-memory game.ActionView for testing
// ActionSpec.Handle implementations without a real *game.GameRoom. It
// intentionally has no concept of *ws.Connection — actions must not be
// able to reach one (see package doc comment) — so a fake at this layer
// cannot even offer one to a misbehaving test.
type fakeActionView struct {
	currentPlayerID    string
	seatedPlayerIDs    []string
	hands              map[string][]cards.Card
	drawPile           []cards.Card
	pendingDraw        map[string]cards.Card
	viewedInitialCards map[string]bool
	advanceTurnCalls   int
	drawTopCardErr     error
	markViewedErr      error
}

func newFakeActionView() *fakeActionView {
	return &fakeActionView{
		hands:              make(map[string][]cards.Card),
		pendingDraw:        make(map[string]cards.Card),
		viewedInitialCards: make(map[string]bool),
	}
}

func (f *fakeActionView) CurrentPlayerID() string {
	return f.currentPlayerID
}

func (f *fakeActionView) AdvanceTurn() {
	f.advanceTurnCalls++
}

func (f *fakeActionView) HandOf(playerID string) ([]cards.Card, error) {
	hand, ok := f.hands[playerID]
	if !ok {
		return nil, fmt.Errorf("fakeActionView: no hand for player %s", playerID)
	}
	return hand, nil
}

func (f *fakeActionView) DrawTopCard(playerID string) (cards.Card, error) {
	if f.drawTopCardErr != nil {
		return cards.Card{}, f.drawTopCardErr
	}
	if len(f.drawPile) == 0 {
		return cards.Card{}, fmt.Errorf("fakeActionView: draw pile empty")
	}
	drawn := f.drawPile[0]
	f.drawPile = f.drawPile[1:]
	f.pendingDraw[playerID] = drawn
	return drawn, nil
}

func (f *fakeActionView) MarkInitialCardsViewed(playerID string) error {
	if f.markViewedErr != nil {
		return f.markViewedErr
	}
	if f.viewedInitialCards[playerID] {
		return fmt.Errorf("fakeActionView: player %s already viewed initial cards", playerID)
	}
	f.viewedInitialCards[playerID] = true
	return nil
}

func (f *fakeActionView) SeatedPlayerIDs() []string {
	return f.seatedPlayerIDs
}
