// Package handoff implements the create/join handshake every new
// WebSocket connection performs before joining the normal game message
// flow (spine AD-9, roomsvc's half of the client-driven handoff). See
// internal/roomsvc/ARCHITECTURE.md for the full message contract this
// package implements.
// handoff follows DIP (dependency inversion principle) it sits above both ws & room_manager
package handoff

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/cabo/cabo-backend/internal/roomsvc/game"
	"github.com/cabo/cabo-backend/internal/roomsvc/ws"
)

const (
	actionCreateRoom = "create_room"
	actionJoinRoom   = "join_room"
)

// request is the one message a client sends right after connecting, to
// say whether it wants to create a new room or join an existing one.
type request struct {
	Action string `json:"action"`
	RoomID string `json:"room_id,omitempty"`
}

// response is the one message Handle sends back: either a success with
// the room ID, or an error with an explanation.
type response struct {
	Status  string `json:"status"`
	RoomID  string `json:"room_id,omitempty"`
	Message string `json:"message,omitempty"`
}

// Handle performs the create/join handshake on conn: it reads exactly one
// message, decides whether the client wants to create a new room or join
// an existing one, talks to roomManager accordingly, and writes back a
// single success or error response.
//
// On success, it returns the player and room the connection joined, so
// the caller can continue the connection's lifetime (e.g. via
// conn.ReadLoop) knowing which room to route further messages to.
//
// On failure, the connection has already been closed — the caller does
// not need to close it again.
func Handle(ctx context.Context, conn *ws.Connection, roomManager *game.RoomManager, log *slog.Logger) (*game.Player, *game.GameRoom, error) {
	// the bllow call is the network & a blocking call
	// TODO: here I was thinking that if 1st message is wrong then client should be given chance to send some more messages or it should be deleted
	data, err := conn.ReadOne(ctx)
	log.Info("First data received is", "data", string(data))
	if err != nil {
		return nil, nil, err
	}

	var req request
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, nil, reject(ctx, conn, log, "invalid request")
	}

	player := game.NewPlayer(conn)
	log.Info("Step 1 : Player created next step going to assign room to it")
	var room *game.GameRoom

	// See here one pattern that in switch statement the error which came it is not checked
	// for each case statement individually instead it is checked once i.e if err != nil {}
	isFull := false
	switch req.Action {
	case actionCreateRoom:
		room, err = roomManager.CreateRoom(player, game.MaxPlayersPerRoom)
		if err == nil {
			isFull = room.IsFull() // covers maxPlayers=1 rooms, already full at creation
		}
	case actionJoinRoom:
		room, err = roomManager.GetRoom(req.RoomID)
		if err == nil {
			isFull, err = room.JoinRoom(player)
		}
	default:
		err = fmt.Errorf("handoff: unknown action %q", req.Action)
	}

	if err != nil {
		return nil, nil, reject(ctx, conn, log, err.Error())
	}

	if isFull {
		if err := room.StartGame(game.CardsPerPlayerAtStart); err != nil {
			log.Error("handoff: failed to start game", "room_id", room.ID, "error", err)
		}
	}

	body, err := json.Marshal(response{Status: "ok", RoomID: room.ID})
	if err != nil {
		return nil, nil, reject(ctx, conn, log, "internal error")
	}
	if err := conn.Write(ctx, body); err != nil {
		return nil, nil, fmt.Errorf("handoff: failed to write success response: %w", err)
	}

	return player, room, nil
}

// reject writes an error response to conn and closes it — the request
// was well-formed enough to evaluate, but rejected (unknown room, full
// room, unknown action, bad JSON) — returning an error describing the
// rejection for Handle's caller.
//
// It closes with CloseNow, not Close: the client already has its answer
// in the JSON body just written, so there is nothing to wait for — Close
// would otherwise block for up to 5s hoping the client sends back its own
// close frame.
func reject(ctx context.Context, conn *ws.Connection, log *slog.Logger, message string) error {
	body, err := json.Marshal(response{Status: "error", Message: message})
	if err != nil {
		log.Error("handoff: failed to encode error response", "error", err)
	} else if err := conn.Write(ctx, body); err != nil {
		log.Warn("handoff: failed to write error response", "error", err)
	}

	conn.CloseNow()
	return fmt.Errorf("handoff: %s", message)
}
