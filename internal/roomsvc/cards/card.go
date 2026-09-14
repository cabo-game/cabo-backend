// Package cards implements standard 52-card deck mechanics: card
// representation, deck construction, shuffling, and dealing. It has no
// knowledge of Cabo's game rules (card values, special powers, turn order)
// — those are still undecided (see cabo-bmad/docs/cabo.md, "To be
// decided") and deliberately not modeled here. Callers in package game
// depend on this package; it depends on nothing in game.
package cards

// Suit is one of the four standard card suits.
type Suit int

const (
	Clubs Suit = iota
	Diamonds
	Hearts
	Spades
)

// Rank is a card's rank, Ace through King. Numeric ranks equal their pip
// count (Two = 2, ..., Ten = 10); Ace, Jack, Queen, King are named
// constants since they have no single numeric pip count. This is purely
// identity — Cabo's scoring value per rank is still undecided (see
// cabo-bmad/docs/cabo.md) and is not represented here.
type Rank int

const (
	Ace Rank = iota + 1
	Two
	Three
	Four
	Five
	Six
	Seven
	Eight
	Nine
	Ten
	Jack
	Queen
	King
)

// Card is one playing card: a rank paired with a suit. JSON-tagged in
// snake_case to match the rest of the gameplay wire contract (player_id,
// room_id, etc.) — see cabo-bmad/_bmad-output/.../BACKEND-CONTRACT-FOR-FRONTEND.md.
type Card struct {
	Rank Rank `json:"rank"`
	Suit Suit `json:"suit"`
}
