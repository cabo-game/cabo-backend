package handoff_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/cabo/cabo-backend/internal/roomsvc/game"
	"github.com/cabo/cabo-backend/internal/roomsvc/handoff"
	"github.com/cabo/cabo-backend/internal/roomsvc/ws"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// testPingConfig returns a PingConfig with an interval long enough that the
// keepalive never fires during a test's short lifetime.
func testPingConfig() ws.PingConfig {
	return ws.PingConfig{Interval: time.Hour, Timeout: time.Minute}
}

// noopNotifier is a test double for authclient.Client that does nothing —
// handoff's tests care about room create/join behavior, not about what
// gets reported to authsvc.
type noopNotifier struct{}

func (noopNotifier) NotifyRoomCreated(roomID string) {}

type response struct {
	Status  string `json:"status"`
	RoomID  string `json:"room_id,omitempty"`
	Message string `json:"message,omitempty"`
}

// dialAndHandle spins up a real WebSocket server whose only job is to run
// handoff.Handle on each accepted connection, and dials a client into it.
// It returns the connected client and a channel carrying Handle's result
// for the one connection under test.
type handleResult struct {
	player *game.Player
	room   *game.GameRoom
	err    error
}

func dialAndHandle(t *testing.T, roomManager *game.RoomManager) (clientConn *websocket.Conn, result <-chan handleResult) {
	t.Helper()

	results := make(chan handleResult, 1)
	h := ws.NewHandler(testLogger(), testPingConfig(), func(c *ws.Connection) {
		player, room, err := handoff.Handle(context.Background(), c, roomManager, testLogger())
		results <- handleResult{player: player, room: room, err: err}
	})
	server := httptest.NewServer(h)
	t.Cleanup(server.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL(server.URL), nil)
	if err != nil {
		t.Fatalf("client dial failed: %v", err)
	}
	t.Cleanup(func() { conn.Close(websocket.StatusNormalClosure, "") })

	return conn, results
}

func sendAndAwaitResult(t *testing.T, conn *websocket.Conn, results <-chan handleResult, request string) handleResult {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := conn.Write(ctx, websocket.MessageText, []byte(request)); err != nil {
		t.Fatalf("client write failed: %v", err)
	}

	select {
	case r := <-results:
		return r
	case <-ctx.Done():
		t.Fatal("timed out waiting for Handle to return")
		return handleResult{}
	}
}

func readResponse(t *testing.T, conn *websocket.Conn) response {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("client read failed: %v", err)
	}

	var resp response
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("response is not valid JSON: %v (body: %s)", err, data)
	}
	return resp
}

func wsURL(httpURL string) string {
	return "ws" + httpURL[len("http"):]
}

func TestHandle_CreateRoom_CreatesRoomAndRespondsSuccess(t *testing.T) {
	roomManager := game.NewRoomManager(testLogger(), noopNotifier{})
	conn, results := dialAndHandle(t, roomManager)

	result := sendAndAwaitResult(t, conn, results, `{"action":"create_room"}`)
	if result.err != nil {
		t.Fatalf("Handle returned error: %v", result.err)
	}
	if result.room == nil {
		t.Fatal("Handle returned a nil room")
	}
	if result.player == nil {
		t.Fatal("Handle returned a nil player")
	}

	resp := readResponse(t, conn)
	if resp.Status != "ok" {
		t.Errorf("response status = %q, want %q", resp.Status, "ok")
	}
	if resp.RoomID != result.room.ID {
		t.Errorf("response room_id = %q, want %q", resp.RoomID, result.room.ID)
	}

	if _, err := roomManager.GetRoom(result.room.ID); err != nil {
		t.Errorf("created room is not retrievable via RoomManager.GetRoom: %v", err)
	}
}

func TestHandle_JoinRoom_JoinsExistingRoomAndRespondsSuccess(t *testing.T) {
	roomManager := game.NewRoomManager(testLogger(), noopNotifier{})
	existingRoom, err := roomManager.CreateRoom(game.NewPlayer(nil), game.MaxPlayersPerRoom)
	if err != nil {
		t.Fatalf("failed to set up an existing room: %v", err)
	}

	conn, results := dialAndHandle(t, roomManager)

	request := `{"action":"join_room","room_id":"` + existingRoom.ID + `"}`
	result := sendAndAwaitResult(t, conn, results, request)
	if result.err != nil {
		t.Fatalf("Handle returned error: %v", result.err)
	}
	if result.room != existingRoom {
		t.Errorf("Handle joined room %v, want the existing room %v", result.room, existingRoom)
	}

	resp := readResponse(t, conn)
	if resp.Status != "ok" {
		t.Errorf("response status = %q, want %q", resp.Status, "ok")
	}
	if resp.RoomID != existingRoom.ID {
		t.Errorf("response room_id = %q, want %q", resp.RoomID, existingRoom.ID)
	}

	if len(existingRoom.Players) != 2 {
		t.Errorf("existing room has %d players, want 2 after join", len(existingRoom.Players))
	}
}

func TestHandle_JoinRoom_UnknownRoomID_RespondsError(t *testing.T) {
	roomManager := game.NewRoomManager(testLogger(), noopNotifier{})
	conn, results := dialAndHandle(t, roomManager)

	result := sendAndAwaitResult(t, conn, results, `{"action":"join_room","room_id":"DOES-NOT-EXIST"}`)
	if result.err == nil {
		t.Fatal("expected Handle to return an error for an unknown room id, got nil")
	}

	resp := readResponse(t, conn)
	if resp.Status != "error" {
		t.Errorf("response status = %q, want %q", resp.Status, "error")
	}
	if resp.Message == "" {
		t.Error("response message is empty, want an explanation")
	}
}

func TestHandle_JoinRoom_RoomFull_RespondsError(t *testing.T) {
	roomManager := game.NewRoomManager(testLogger(), noopNotifier{})
	fullRoom, err := roomManager.CreateRoom(game.NewPlayer(nil), 1)
	if err != nil {
		t.Fatalf("failed to set up a full room: %v", err)
	}

	conn, results := dialAndHandle(t, roomManager)

	request := `{"action":"join_room","room_id":"` + fullRoom.ID + `"}`
	result := sendAndAwaitResult(t, conn, results, request)
	if result.err == nil {
		t.Fatal("expected Handle to return an error for a full room, got nil")
	}

	resp := readResponse(t, conn)
	if resp.Status != "error" {
		t.Errorf("response status = %q, want %q", resp.Status, "error")
	}
}

func TestHandle_UnknownAction_RespondsError(t *testing.T) {
	roomManager := game.NewRoomManager(testLogger(), noopNotifier{})
	conn, results := dialAndHandle(t, roomManager)

	result := sendAndAwaitResult(t, conn, results, `{"action":"do_a_barrel_roll"}`)
	if result.err == nil {
		t.Fatal("expected Handle to return an error for an unknown action, got nil")
	}

	resp := readResponse(t, conn)
	if resp.Status != "error" {
		t.Errorf("response status = %q, want %q", resp.Status, "error")
	}
}

func TestHandle_InvalidJSON_RespondsError(t *testing.T) {
	roomManager := game.NewRoomManager(testLogger(), noopNotifier{})
	conn, results := dialAndHandle(t, roomManager)

	result := sendAndAwaitResult(t, conn, results, `not json at all`)
	if result.err == nil {
		t.Fatal("expected Handle to return an error for invalid JSON, got nil")
	}

	resp := readResponse(t, conn)
	if resp.Status != "error" {
		t.Errorf("response status = %q, want %q", resp.Status, "error")
	}
}
