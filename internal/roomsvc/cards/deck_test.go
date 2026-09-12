package cards

import "testing"

func TestNewDeck_Has52Cards(t *testing.T) {
	deck := NewDeck()

	if len(deck) != 52 {
		t.Fatalf("len(NewDeck()) = %d, want 52", len(deck))
	}
}

func TestNewDeck_AllCardsAreUnique(t *testing.T) {
	deck := NewDeck()

	seen := make(map[Card]bool, len(deck))
	for _, c := range deck {
		if seen[c] {
			t.Fatalf("duplicate card in deck: %+v", c)
		}
		seen[c] = true
	}
}

func TestNewDeck_HasFourSuitsAndThirteenRanksEach(t *testing.T) {
	deck := NewDeck()

	countBySuit := make(map[Suit]int)
	for _, c := range deck {
		countBySuit[c.Suit]++
	}

	if len(countBySuit) != 4 {
		t.Fatalf("distinct suits = %d, want 4", len(countBySuit))
	}
	for suit, count := range countBySuit {
		if count != 13 {
			t.Errorf("suit %v has %d cards, want 13", suit, count)
		}
	}
}

func TestShuffle_PreservesTheSameCards(t *testing.T) {
	deck := NewDeck()
	original := make(map[Card]int, len(deck))
	for _, c := range deck {
		original[c]++
	}

	Shuffle(deck)

	if len(deck) != 52 {
		t.Fatalf("len(deck) after Shuffle = %d, want 52", len(deck))
	}
	after := make(map[Card]int, len(deck))
	for _, c := range deck {
		after[c]++
	}
	for c, count := range original {
		if after[c] != count {
			t.Errorf("card %+v appears %d times after shuffle, want %d", c, after[c], count)
		}
	}
}

func TestShuffle_ChangesOrder(t *testing.T) {
	// Not a strict guarantee (a shuffle could land on the original order
	// by chance), but with 52! possible orderings, retrying a handful of
	// times makes a false failure astronomically unlikely.
	deck := NewDeck()
	original := make([]Card, len(deck))
	copy(original, deck)

	changed := false
	for attempt := 0; attempt < 5 && !changed; attempt++ {
		Shuffle(deck)
		for i := range deck {
			if deck[i] != original[i] {
				changed = true
				break
			}
		}
	}

	if !changed {
		t.Error("Shuffle produced the same order as the original deck across 5 attempts")
	}
}
