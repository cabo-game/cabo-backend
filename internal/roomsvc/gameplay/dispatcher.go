// Package gameplay wires incoming WebSocket messages from a seated player
// to the game-rule logic in package gameplay/actions, and delivers each
// action's result back over the wire.
//
// This is the only package that imports both internal/roomsvc/ws and
// internal/roomsvc/gameplay/actions together — actions itself never
// imports ws (see actions' package doc comment). That split is what makes
// "an action handler cannot write to a socket" a compile-time fact:
// Dispatcher is the sole place a Recipient.PlayerID is resolved to a live
// *ws.Connection (via GameRoom.PlayerConn) and Write is called.
package gameplay

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/cabo/cabo-backend/internal/roomsvc/game"
	"github.com/cabo/cabo-backend/internal/roomsvc/gameplay/actions"
)

// incomingMessage is the wire shape of a gameplay message: which action
// the client wants to perform, plus whatever the action itself needs
// (unmarshaled separately by each ActionSpec.Handle from the raw bytes,
// same pattern as internal/roomsvc/handoff's request/response structs).
type incomingMessage struct {
	Action string `json:"action"`
}

// Dispatcher routes one seated player's messages to the matching
// ActionSpec and delivers the result. Stateless — the action registry is
// a package-level, read-only map — so one Dispatcher can be shared across
// every connection.
type Dispatcher struct {
	log *slog.Logger
}

// NewDispatcher creates a Dispatcher. log is used for delivery failures
// (a write to a disconnected recipient) that can't be reported back to
// the acting player, since their own reply may already be part of the
// same failed delivery batch.
func NewDispatcher(log *slog.Logger) *Dispatcher {
	return &Dispatcher{log: log}
}

// OnMessage handles one raw WebSocket message from actingPlayerID, seated
// in room. It looks up the requested action, checks turn eligibility,
// runs the action, and writes each resulting PlayerView to its recipient.
//
// Matches the func(data []byte) shape ws.Connection.ReadLoop expects for
// onMessage once curried with a specific room/player — see
// cmd/roomsvc/main.go's onConnect, which does the same currying for
// handoff.Handle's onClose callback today.
func (d *Dispatcher) OnMessage(room *game.GameRoom, actingPlayerID string, raw []byte) {
	var msg incomingMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		d.replyError(room, actingPlayerID, "invalid message")
		return
	}

	spec, ok := actionRegistry[msg.Action]
	if !ok {
		d.replyError(room, actingPlayerID, fmt.Sprintf("unknown action %q", msg.Action))
		return
	}

	if err := checkTurn(spec, actingPlayerID, room.CurrentPlayerID()); err != nil {
		d.replyError(room, actingPlayerID, err.Error())
		return
	}

	effect, err := spec.Handle(actions.ActionContext{
		Room:           room,
		ActingPlayerID: actingPlayerID,
		Payload:        raw,
	})
	if err != nil {
		d.replyError(room, actingPlayerID, err.Error())
		return
	}

	d.deliver(room, effect)
}

// replyError sends a single error PlayerView directly to actingPlayerID —
// there is no ActionSpec involved (the message never resolved to one, or
// it was rejected before Handle ran), so there is no Effect to route
// through deliver.
func (d *Dispatcher) replyError(room *game.GameRoom, actingPlayerID, message string) {
	payload, err := json.Marshal(struct {
		Message string `json:"message"`
	}{Message: message})
	if err != nil {
		d.log.Error("gameplay: failed to encode error payload", "error", err)
		return
	}
	d.deliver(room, actions.Effect{
		Recipients: []actions.Recipient{
			{PlayerID: actingPlayerID, View: actions.PlayerView{Event: "error", Payload: payload}},
		},
	})
}

// deliver resolves each Recipient.PlayerID to a live *ws.Connection via
// GameRoom.PlayerConn (lock-guarded — see its doc comment) and writes its
// PlayerView. This is the only place in the gameplay message path that
// touches a *ws.Connection — see the package doc comment.
//
// A recipient no longer seated (e.g. disconnected between the action
// running and delivery) or a write failure is logged, not returned: by
// this point the action has already mutated room state, so there is no
// single caller left to report a delivery failure to as an error.
func (d *Dispatcher) deliver(room *game.GameRoom, effect actions.Effect) {
	for _, recipient := range effect.Recipients {
		conn, ok := room.PlayerConn(recipient.PlayerID)
		if !ok {
			d.log.Warn("gameplay: recipient no longer seated, dropping message", "room_id", room.ID, "player_id", recipient.PlayerID)
			continue
		}

		body, err := json.Marshal(recipient.View)
		if err != nil {
			d.log.Error("gameplay: failed to encode player view", "room_id", room.ID, "player_id", recipient.PlayerID, "error", err)
			continue
		}

		if err := conn.Write(context.Background(), body); err != nil {
			d.log.Warn("gameplay: failed to deliver message", "room_id", room.ID, "player_id", recipient.PlayerID, "error", err)
		}
	}
}
