package cards

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

// suits and ranks are the fixed 4x13 composition of a standard deck (see
// cabo-bmad/docs/cabo.md, "Deck size: standard 52-card deck").
var suits = []Suit{Clubs, Diamonds, Hearts, Spades}
var ranks = []Rank{Ace, Two, Three, Four, Five, Six, Seven, Eight, Nine, Ten, Jack, Queen, King}

// NewDeck returns a standard 52-card deck (4 suits x 13 ranks, no jokers)
// in a fixed, unshuffled order. Callers that need randomness call Shuffle
// on the result.
func NewDeck() []Card {
	deck := make([]Card, 0, len(suits)*len(ranks))
	for _, suit := range suits {
		for _, rank := range ranks {
			deck = append(deck, Card{Rank: rank, Suit: suit})
		}
	}
	return deck
}

// Shuffle randomizes deck in place using the Fisher-Yates algorithm.
// It uses crypto/rand rather than math/rand: this is a card game, and a
// predictable shuffle would let a player predict the deck's remaining
// order (the same reasoning this codebase already applies to room codes
// and player IDs).
func Shuffle(deck []Card) {
	for i := len(deck) - 1; i > 0; i-- {
		j := randIntn(i + 1)
		deck[i], deck[j] = deck[j], deck[i]
	}
}

// randIntn returns a cryptographically random integer in [0, n).
func randIntn(n int) int {
	max := big.NewInt(int64(n))
	v, err := rand.Int(rand.Reader, max)
	if err != nil {
		panic(fmt.Sprintf("roomsvc: failed to read random bytes for shuffle: %v", err))
	}
	return int(v.Int64())
}
