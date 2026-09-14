// Package actions implements Cabo gameplay actions: the game-rule logic
// that decides how one player's message mutates room state and what each
// affected player is told about it.
//
// This package deliberately does not import internal/roomsvc/ws. Every
// action's Handle only ever holds an ActionContext, whose Room field is
// game.ActionView — a narrow interface with no path to a live
// *ws.Connection (see game.ActionView's doc comment). This makes "an
// action cannot write to a socket" a compile-time fact: package
// internal/roomsvc/gameplay (one level up) is the only place that imports
// both ws and actions, and is therefore the only place allowed to resolve
// a Recipient.PlayerID to a live connection and call Write.
package actions

import (
	"encoding/json"

	"github.com/cabo/cabo-backend/internal/roomsvc/game"
)

// ActionSpec is one gameplay action a player's message can invoke — e.g.
// drawing a card. Implementations are stateless: all per-request data
// arrives through ActionContext, not through the ActionSpec value itself.
type ActionSpec interface {
	// Name is the wire "action" string that selects this spec, e.g.
	// "draw_card". Matches the key this spec is registered under in
	// package gameplay's action registry.
	Name() string

	// RequiresCurrentTurn reports whether this action may only be
	// performed by the room's current-turn player. Cabo allows some
	// actions (e.g. a future snap/slap reaction) from any seated player
	// regardless of whose turn it is — those specs return false here.
	RequiresCurrentTurn() bool

	// Handle applies this action's effect to ctx.Room and returns who
	// should be told what. Handle never writes to a connection itself —
	// see the package doc comment for why that isn't possible from here.
	Handle(ctx ActionContext) (Effect, error)
}

// ActionContext is the per-request data passed to one ActionSpec.Handle
// call. Unlike ActionSpec, this is never static — every message from
// every player produces a new ActionContext.
type ActionContext struct {
	// Room is the acting player's room, narrowed to game.ActionView so
	// Handle cannot reach Players[i].Conn (see package doc comment).
	Room game.ActionView

	// ActingPlayerID is the ID of the player who sent this message.
	ActingPlayerID string

	// Payload is the action-specific portion of the incoming message
	// (e.g. which card index to swap), still encoded as raw JSON — each
	// ActionSpec unmarshals only the shape it expects.
	Payload json.RawMessage
}

// Effect is the result of one ActionSpec.Handle call: who should be told
// something, and what each of them should be told. It carries no raw game
// state — every PlayerView in it is already the exact, redacted payload
// safe to send to that one recipient.
type Effect struct {
	Recipients []Recipient
}

// Recipient pairs a player with the message they should receive.
type Recipient struct {
	PlayerID string
	View     PlayerView
}

// PlayerView is a wire-ready payload: exactly what gets marshalled and
// written to one recipient's connection, nothing else. Any reasoning
// about what is safe to reveal to this specific recipient (e.g. "the
// drawn card's face is only visible to the player who drew it") happens
// inside the ActionSpec that builds the PlayerView, not as fields carried
// on this struct — there is nothing left to compute once a PlayerView
// exists.
type PlayerView struct {
	Event   string          `json:"event"`
	Payload json.RawMessage `json:"payload,omitempty"`
}
