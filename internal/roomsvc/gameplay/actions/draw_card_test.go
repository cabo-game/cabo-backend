package actions

import (
	"encoding/json"
	"testing"

	"github.com/cabo/cabo-backend/internal/roomsvc/cards"
)

func TestDrawCardAction_Name(t *testing.T) {
	if got := (DrawCardAction{}).Name(); got != "draw_card" {
		t.Errorf("Name() = %q, want %q", got, "draw_card")
	}
}

func TestDrawCardAction_RequiresCurrentTurn(t *testing.T) {
	if !(DrawCardAction{}).RequiresCurrentTurn() {
		t.Error("RequiresCurrentTurn() = false, want true (drawing is a turn-gated action)")
	}
}

func TestDrawCardAction_Handle_DrawerRecipientSeesTheCard(t *testing.T) {
	view := newFakeActionView()
	view.currentPlayerID = "player-1"
	view.seatedPlayerIDs = []string{"player-1", "player-2", "player-3"}
	view.drawPile = []cards.Card{{Rank: cards.Seven, Suit: cards.Hearts}}

	effect, err := (DrawCardAction{}).Handle(ActionContext{
		Room:           view,
		ActingPlayerID: "player-1",
	})
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	recipient := findRecipient(t, effect, "player-1")
	if recipient.View.Event != "you_drew" {
		t.Errorf("recipient.View.Event = %q, want %q", recipient.View.Event, "you_drew")
	}

	var payload struct {
		Card cards.Card `json:"card"`
	}
	if err := json.Unmarshal(recipient.View.Payload, &payload); err != nil {
		t.Fatalf("failed to unmarshal recipient.View.Payload: %v", err)
	}
	wantCard := cards.Card{Rank: cards.Seven, Suit: cards.Hearts}
	if payload.Card != wantCard {
		t.Errorf("payload.Card = %+v, want %+v", payload.Card, wantCard)
	}
}

func TestDrawCardAction_Handle_OtherPlayersNotifiedWithoutSeeingTheCard(t *testing.T) {
	view := newFakeActionView()
	view.currentPlayerID = "player-1"
	view.seatedPlayerIDs = []string{"player-1", "player-2", "player-3"}
	view.drawPile = []cards.Card{{Rank: cards.Seven, Suit: cards.Hearts}}

	effect, err := (DrawCardAction{}).Handle(ActionContext{
		Room:           view,
		ActingPlayerID: "player-1",
	})
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	if len(effect.Recipients) != 3 {
		t.Fatalf("len(effect.Recipients) = %d, want 3 (drawer + 2 other seated players)", len(effect.Recipients))
	}

	for _, id := range []string{"player-2", "player-3"} {
		recipient := findRecipient(t, effect, id)
		if recipient.View.Event != "player_drew" {
			t.Errorf("recipient[%s].View.Event = %q, want %q", id, recipient.View.Event, "player_drew")
		}

		var payload map[string]json.RawMessage
		if err := json.Unmarshal(recipient.View.Payload, &payload); err != nil {
			t.Fatalf("failed to unmarshal recipient[%s].View.Payload: %v", id, err)
		}
		if _, hasCard := payload["card"]; hasCard {
			t.Errorf("recipient[%s].View.Payload contains a \"card\" field, want none (card must not be visible to other players)", id)
		}

		var playerID string
		if err := json.Unmarshal(payload["player_id"], &playerID); err != nil {
			t.Fatalf("failed to unmarshal recipient[%s] player_id: %v", id, err)
		}
		if playerID != "player-1" {
			t.Errorf("recipient[%s] payload player_id = %q, want %q", id, playerID, "player-1")
		}
	}
}

// findRecipient returns the Recipient for playerID, failing the test if
// none is present.
func findRecipient(t *testing.T, effect Effect, playerID string) Recipient {
	t.Helper()
	for _, r := range effect.Recipients {
		if r.PlayerID == playerID {
			return r
		}
	}
	t.Fatalf("no recipient for player %s in %+v", playerID, effect.Recipients)
	return Recipient{}
}

func TestDrawCardAction_Handle_DoesNotAdvanceTurn(t *testing.T) {
	view := newFakeActionView()
	view.currentPlayerID = "player-1"
	view.seatedPlayerIDs = []string{"player-1"}
	view.drawPile = []cards.Card{{Rank: cards.Two, Suit: cards.Clubs}}

	if _, err := (DrawCardAction{}).Handle(ActionContext{Room: view, ActingPlayerID: "player-1"}); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	if view.advanceTurnCalls != 0 {
		t.Errorf("AdvanceTurn called %d times, want 0 (the acting player still has to resolve the drawn card before their turn ends)", view.advanceTurnCalls)
	}
}

func TestDrawCardAction_Handle_PropagatesDrawError(t *testing.T) {
	view := newFakeActionView()
	view.currentPlayerID = "player-1"
	// No cards in drawPile: DrawTopCard errors.

	_, err := (DrawCardAction{}).Handle(ActionContext{Room: view, ActingPlayerID: "player-1"})
	if err == nil {
		t.Fatal("expected an error when the draw pile is empty, got nil")
	}
}
