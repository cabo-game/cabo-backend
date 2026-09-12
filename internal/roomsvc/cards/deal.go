package cards

import "fmt"

// Deal distributes cardsPerHand cards to each of numHands hands, taking
// cards from the front of deck in contiguous blocks: the first
// cardsPerHand cards go to hand 0, the next cardsPerHand to hand 1, and so
// on. deck is expected to already be shuffled — Deal does not shuffle, so
// which block a hand gets carries no bias only because the deck order
// itself is already random.
//
// It returns the dealt hands and whatever is left of deck as the
// remaining draw pile. deck is not modified; the returned slices are new.
//
// Deal returns an error, dealing nothing, if deck does not have enough
// cards for numHands * cardsPerHand.
func Deal(deck []Card, numHands, cardsPerHand int) (hands [][]Card, remaining []Card, err error) {
	needed := numHands * cardsPerHand
	if len(deck) < needed {
		return nil, nil, fmt.Errorf("cards: not enough cards to deal: have %d, need %d (%d hands x %d cards)", len(deck), needed, numHands, cardsPerHand)
	}

	hands = make([][]Card, numHands)
	next := 0
	for h := 0; h < numHands; h++ {
		hand := make([]Card, cardsPerHand)
		copy(hand, deck[next:next+cardsPerHand])
		hands[h] = hand
		next += cardsPerHand
	}

	remaining = make([]Card, len(deck)-next)
	copy(remaining, deck[next:])

	return hands, remaining, nil
}
