package ws_test

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/cabo/cabo-backend/internal/roomsvc/ws"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// testPingConfig returns a PingConfig with an interval long enough that the
// keepalive never fires during a test's short lifetime, so ping/pong
// behavior doesn't need to be considered by tests that aren't targeting it.
func testPingConfig() ws.PingConfig {
	return ws.PingConfig{Interval: time.Hour, Timeout: time.Minute}
}

func TestHandler_AcceptsWebSocketUpgrade(t *testing.T) {
	connected := make(chan *ws.Connection, 1)
	handler := ws.NewHandler(testLogger(), testPingConfig(), func(c *ws.Connection) {
		connected <- c
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	clientConn, _, err := websocket.Dial(ctx, wsURL(server.URL), nil)
	if err != nil {
		t.Fatalf("client dial failed: %v", err)
	}
	defer clientConn.Close(websocket.StatusNormalClosure, "")

	select {
	case c := <-connected:
		if c.RemoteAddr() == "" {
			t.Error("expected non-empty remote address")
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for OnConnect to fire")
	}
}

func TestHandler_RejectsPlainHTTPRequest(t *testing.T) {
	handler := ws.NewHandler(testLogger(), testPingConfig(), func(c *ws.Connection) {
		t.Error("OnConnect should not fire for a non-upgrade request")
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	resp, err := server.Client().Get(server.URL)
	if err != nil {
		t.Fatalf("plain GET failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 101 {
		t.Errorf("expected upgrade to be rejected, got status %d", resp.StatusCode)
	}
}

func wsURL(httpURL string) string {
	return "ws" + httpURL[len("http"):]
}
