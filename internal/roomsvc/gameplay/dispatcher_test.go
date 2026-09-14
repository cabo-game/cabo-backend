package gameplay_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/cabo/cabo-backend/internal/roomsvc/game"
	"github.com/cabo/cabo-backend/internal/roomsvc/gameplay"
	"github.com/cabo/cabo-backend/internal/roomsvc/gameplay/actions"
	"github.com/cabo/cabo-backend/internal/roomsvc/ws"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testPingConfig() ws.PingConfig {
	return ws.PingConfig{Interval: time.Hour, Timeout: time.Minute}
}

func wsURL(httpURL string) string {
	u, err := url.Parse(httpURL)
	if err != nil {
		panic(err)
	}
	u.Scheme = "ws"
	return u.String()
}

// dialPlayer starts a real WebSocket server for player (seated in room,
// via game.Player.Conn — this is the one thing this test package is
// allowed to set directly, since here it plays the role of what
// cmd/roomsvc/main.go's onConnect normally does when handoff seats a real
// connection). It hands every accepted connection to
// gameplay.NewDispatcher(...).OnMessage and dials one client into it,
// exercising the full path including the real *ws.Connection.Write calls
// Dispatcher makes when delivering PlayerViews.
//
// dialPlayer blocks until player.Conn has actually been assigned by the
// server's onConnect goroutine before returning. websocket.Dial returning
// only means the client's half of the handshake finished — the server's
// onConnect callback runs in its own goroutine and could still be
// in-flight. Without waiting for it here, a second dialPlayer call could
// return to its caller before player.Conn is actually set, and a message
// sent on another connection could race Dispatcher.deliver reading that
// field concurrently with this goroutine writing it (a real data race,
// caught by `go test -race`). Production code has no equivalent gap: a
// real Player is only ever seated with its Conn already fully set (see
// game.NewPlayer), so this synchronization only exists because this test
// deliberately seats a placeholder Player before its connection exists.
func dialPlayer(t *testing.T, room *game.GameRoom, player *game.Player) *websocket.Conn {
	t.Helper()

	dispatcher := gameplay.NewDispatcher(testLogger())
	connected := make(chan struct{})

	h := ws.NewHandler(testLogger(), testPingConfig(), func(c *ws.Connection) {
		player.Conn = c
		close(connected)
		c.ReadLoop(context.Background(), func(data []byte) {
			dispatcher.OnMessage(room, player.ID, data)
		}, func() {})
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

	select {
	case <-connected:
	case <-ctx.Done():
		t.Fatal("timed out waiting for server to accept connection")
	}

	return conn
}

func sendMessage(t *testing.T, conn *websocket.Conn, body string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := conn.Write(ctx, websocket.MessageText, []byte(body)); err != nil {
		t.Fatalf("client write failed: %v", err)
	}
}

// readView reads and decodes one message using actions.PlayerView — the
// same struct the server marshals a response from — so this test verifies
// against the real wire contract instead of a hand-maintained duplicate
// that could silently drift from it.
func readView(t *testing.T, conn *websocket.Conn) actions.PlayerView {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("client read failed: %v", err)
	}

	var v actions.PlayerView
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("response is not valid JSON: %v (body: %s)", err, data)
	}
	return v
}

// newTwoPlayerStartedRoom builds a room directly via game.NewGameRoom /
// JoinRoom / StartGame, bypassing game.RoomManager: these tests exercise
// post-handoff message dispatch against a room already in a known state,
// not room creation/registration, which is RoomManager's own concern
// (and its own test file). game_room_test.go follows the same pattern.
func newTwoPlayerStartedRoom(t *testing.T) (*game.GameRoom, *game.Player, *game.Player) {
	t.Helper()

	first := &game.Player{ID: "player-1"}
	room, err := game.NewGameRoom(first, 2, testLogger())
	if err != nil {
		t.Fatalf("NewGameRoom returned error: %v", err)
	}
	second := &game.Player{ID: "player-2"}
	if _, err := room.JoinRoom(second); err != nil {
		t.Fatalf("JoinRoom returned error: %v", err)
	}
	if err := room.StartGame(game.CardsPerPlayerAtStart); err != nil {
		t.Fatalf("StartGame returned error: %v", err)
	}
	return room, first, second
}

func TestDispatcher_OnMessage_ViewInitialCards_RepliesToActingPlayerOnly(t *testing.T) {
	room, first, _ := newTwoPlayerStartedRoom(t)
	conn := dialPlayer(t, room, first)

	sendMessage(t, conn, `{"action":"view_initial_cards"}`)

	view := readView(t, conn)
	if view.Event != "initial_cards" {
		t.Errorf("view.Event = %q, want %q", view.Event, "initial_cards")
	}
}

func TestDispatcher_OnMessage_DrawCard_CurrentPlayerSucceeds(t *testing.T) {
	room, first, second := newTwoPlayerStartedRoom(t)
	conn := dialPlayer(t, room, first)
	// A successful draw also notifies every other seated player (see
	// TestDispatcher_OnMessage_DrawCard_OtherSeatedPlayerAlsoNotified) —
	// second must be dialed too, or PlayerConn finds no live connection
	// to deliver that notification to, same as it would for any other
	// player who has genuinely never connected.
	otherConn := dialPlayer(t, room, second)

	sendMessage(t, conn, `{"action":"draw_card"}`)

	view := readView(t, conn)
	if view.Event != "you_drew" {
		t.Errorf("view.Event = %q, want %q", view.Event, "you_drew")
	}
	readView(t, otherConn) // drain second's "player_drew" notification
}

func TestDispatcher_OnMessage_DrawCard_RejectsNonCurrentPlayer(t *testing.T) {
	room, _, second := newTwoPlayerStartedRoom(t)
	conn := dialPlayer(t, room, second) // player-1 is current, not second

	sendMessage(t, conn, `{"action":"draw_card"}`)

	view := readView(t, conn)
	if view.Event != "error" {
		t.Errorf("view.Event = %q, want %q (not this player's turn)", view.Event, "error")
	}
	if len(second.Hand) != game.CardsPerPlayerAtStart || second.DrawnCard != nil {
		t.Error("rejected draw must not mutate the acting player's state")
	}
}

func TestDispatcher_OnMessage_UnknownAction_RepliesWithError(t *testing.T) {
	room, first, _ := newTwoPlayerStartedRoom(t)
	conn := dialPlayer(t, room, first)

	sendMessage(t, conn, `{"action":"does_not_exist"}`)

	view := readView(t, conn)
	if view.Event != "error" {
		t.Errorf("view.Event = %q, want %q", view.Event, "error")
	}
}

func TestDispatcher_OnMessage_InvalidJSON_RepliesWithError(t *testing.T) {
	room, first, _ := newTwoPlayerStartedRoom(t)
	conn := dialPlayer(t, room, first)

	sendMessage(t, conn, `not json`)

	view := readView(t, conn)
	if view.Event != "error" {
		t.Errorf("view.Event = %q, want %q", view.Event, "error")
	}
}

func TestDispatcher_OnMessage_DrawCard_OtherSeatedPlayerAlsoNotified(t *testing.T) {
	room, first, second := newTwoPlayerStartedRoom(t)
	firstConn := dialPlayer(t, room, first)
	secondConn := dialPlayer(t, room, second)

	sendMessage(t, firstConn, `{"action":"draw_card"}`)

	drawerView := readView(t, firstConn)
	if drawerView.Event != "you_drew" {
		t.Fatalf("drawer view.Event = %q, want %q", drawerView.Event, "you_drew")
	}

	otherView := readView(t, secondConn)
	if otherView.Event != "player_drew" {
		t.Errorf("other player's view.Event = %q, want %q", otherView.Event, "player_drew")
	}
	if strings.Contains(string(otherView.Payload), "card") {
		t.Errorf("other player's payload leaks card info: %s", otherView.Payload)
	}
}
