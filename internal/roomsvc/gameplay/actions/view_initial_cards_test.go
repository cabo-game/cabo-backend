package actions

import (
	"encoding/json"
	"testing"

	"github.com/cabo/cabo-backend/internal/roomsvc/cards"
)

func TestViewInitialCardsAction_Name(t *testing.T) {
	if got := (ViewInitialCardsAction{}).Name(); got != "view_initial_cards" {
		t.Errorf("Name() = %q, want %q", got, "view_initial_cards")
	}
}

func TestViewInitialCardsAction_RequiresCurrentTurn(t *testing.T) {
	if (ViewInitialCardsAction{}).RequiresCurrentTurn() {
		t.Error("RequiresCurrentTurn() = true, want false (the initial peek is not turn-gated)")
	}
}

func TestViewInitialCardsAction_Handle_RevealsFirstTwoCardsToViewerOnly(t *testing.T) {
	view := newFakeActionView()
	view.hands["player-1"] = []cards.Card{
		{Rank: cards.Ace, Suit: cards.Spades},
		{Rank: cards.King, Suit: cards.Diamonds},
		{Rank: cards.Three, Suit: cards.Clubs},
		{Rank: cards.Nine, Suit: cards.Hearts},
	}

	effect, err := (ViewInitialCardsAction{}).Handle(ActionContext{
		Room:           view,
		ActingPlayerID: "player-1",
	})
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	if len(effect.Recipients) != 1 {
		t.Fatalf("len(effect.Recipients) = %d, want 1 (viewer only)", len(effect.Recipients))
	}
	recipient := effect.Recipients[0]
	if recipient.PlayerID != "player-1" {
		t.Errorf("recipient.PlayerID = %q, want %q", recipient.PlayerID, "player-1")
	}
	if recipient.View.Event != "initial_cards" {
		t.Errorf("recipient.View.Event = %q, want %q", recipient.View.Event, "initial_cards")
	}

	var payload struct {
		Cards []cards.Card `json:"cards"`
	}
	if err := json.Unmarshal(recipient.View.Payload, &payload); err != nil {
		t.Fatalf("failed to unmarshal recipient.View.Payload: %v", err)
	}
	wantCards := []cards.Card{
		{Rank: cards.Ace, Suit: cards.Spades},
		{Rank: cards.King, Suit: cards.Diamonds},
	}
	if len(payload.Cards) != len(wantCards) {
		t.Fatalf("len(payload.Cards) = %d, want %d", len(payload.Cards), len(wantCards))
	}
	for i, c := range wantCards {
		if payload.Cards[i] != c {
			t.Errorf("payload.Cards[%d] = %+v, want %+v", i, payload.Cards[i], c)
		}
	}
}

func TestViewInitialCardsAction_Handle_MarksInitialCardsViewed(t *testing.T) {
	view := newFakeActionView()
	view.hands["player-1"] = []cards.Card{
		{Rank: cards.Ace, Suit: cards.Spades},
		{Rank: cards.King, Suit: cards.Diamonds},
	}

	if _, err := (ViewInitialCardsAction{}).Handle(ActionContext{Room: view, ActingPlayerID: "player-1"}); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	if !view.viewedInitialCards["player-1"] {
		t.Error("MarkInitialCardsViewed was not called for player-1")
	}
}

func TestViewInitialCardsAction_Handle_PropagatesMarkViewedError(t *testing.T) {
	view := newFakeActionView()
	view.hands["player-1"] = []cards.Card{
		{Rank: cards.Ace, Suit: cards.Spades},
		{Rank: cards.King, Suit: cards.Diamonds},
	}
	view.viewedInitialCards["player-1"] = true // already viewed

	_, err := (ViewInitialCardsAction{}).Handle(ActionContext{Room: view, ActingPlayerID: "player-1"})
	if err == nil {
		t.Error("expected an error on a second view_initial_cards call, got nil")
	}
}

func TestViewInitialCardsAction_Handle_PropagatesHandOfError(t *testing.T) {
	view := newFakeActionView()
	// No hand registered for "player-1": HandOf errors.

	_, err := (ViewInitialCardsAction{}).Handle(ActionContext{Room: view, ActingPlayerID: "player-1"})
	if err == nil {
		t.Error("expected an error when HandOf fails, got nil")
	}
}
