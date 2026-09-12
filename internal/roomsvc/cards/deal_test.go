package cards

import (
	"reflect"
	"testing"
)

func card(rank Rank) Card {
	return Card{Rank: rank, Suit: Clubs}
}

func TestDeal_GivesEachHandTheRightNumberOfCards(t *testing.T) {
	deck := NewDeck() // 52 cards

	hands, remaining, err := Deal(deck, 4, 4)
	if err != nil {
		t.Fatalf("Deal returned error: %v", err)
	}

	if len(hands) != 4 {
		t.Fatalf("len(hands) = %d, want 4", len(hands))
	}
	for i, h := range hands {
		if len(h) != 4 {
			t.Errorf("len(hands[%d]) = %d, want 4", i, len(h))
		}
	}
	if len(remaining) != 52-16 {
		t.Errorf("len(remaining) = %d, want %d", len(remaining), 52-16)
	}
}

func TestDeal_DealsContiguousBlocksInOrder(t *testing.T) {
	deck := []Card{
		card(Ace), card(Two), card(Three), card(Four),
		card(Five), card(Six), card(Seven), card(Eight),
		card(Nine),
	}

	hands, remaining, err := Deal(deck, 2, 3)
	if err != nil {
		t.Fatalf("Deal returned error: %v", err)
	}

	wantHand0 := []Card{card(Ace), card(Two), card(Three)}
	wantHand1 := []Card{card(Four), card(Five), card(Six)}
	wantRemaining := []Card{card(Seven), card(Eight), card(Nine)}

	if !reflect.DeepEqual(hands[0], wantHand0) {
		t.Errorf("hands[0] = %v, want %v", hands[0], wantHand0)
	}
	if !reflect.DeepEqual(hands[1], wantHand1) {
		t.Errorf("hands[1] = %v, want %v", hands[1], wantHand1)
	}
	if !reflect.DeepEqual(remaining, wantRemaining) {
		t.Errorf("remaining = %v, want %v", remaining, wantRemaining)
	}
}

func TestDeal_ErrorsWhenNotEnoughCards(t *testing.T) {
	deck := NewDeck() // 52 cards, need 5*11=55

	_, _, err := Deal(deck, 5, 11)
	if err == nil {
		t.Fatal("expected an error when the deck has fewer cards than needed, got nil")
	}
}

func TestDeal_DoesNotModifyInputDeck(t *testing.T) {
	deck := NewDeck()
	original := make([]Card, len(deck))
	copy(original, deck)

	if _, _, err := Deal(deck, 4, 4); err != nil {
		t.Fatalf("Deal returned error: %v", err)
	}

	if !reflect.DeepEqual(deck, original) {
		t.Error("Deal modified its input deck, want it left unchanged")
	}
}
