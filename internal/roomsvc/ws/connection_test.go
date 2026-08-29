package ws_test

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/cabo/cabo-backend/internal/roomsvc/ws"
)

func TestConnection_ReadLoop_DeliversMessagesToCallback(t *testing.T) {
	received := make(chan []byte, 1)
	handler := ws.NewHandler(testLogger(), testPingConfig(), func(c *ws.Connection) {
		c.ReadLoop(context.Background(), func(data []byte) {
			received <- data
		}, func() {})
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

	want := "hello room"
	if err := clientConn.Write(ctx, websocket.MessageText, []byte(want)); err != nil {
		t.Fatalf("client write failed: %v", err)
	}

	select {
	case got := <-received:
		if string(got) != want {
			t.Errorf("onMessage got %q, want %q", got, want)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for message to reach onMessage")
	}
}

func TestConnection_Write_DeliversMessageToClient(t *testing.T) {
	serverConnReady := make(chan *ws.Connection, 1)
	handler := ws.NewHandler(testLogger(), testPingConfig(), func(c *ws.Connection) {
		serverConnReady <- c
		c.ReadLoop(context.Background(), func(data []byte) {}, func() {})
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

	var serverConn *ws.Connection
	select {
	case serverConn = <-serverConnReady:
	case <-ctx.Done():
		t.Fatal("timed out waiting for server connection")
	}

	want := "hello client"
	if err := serverConn.Write(ctx, []byte(want)); err != nil {
		t.Fatalf("server write failed: %v", err)
	}

	msgType, got, err := clientConn.Read(ctx)
	if err != nil {
		t.Fatalf("client read failed: %v", err)
	}
	if msgType != websocket.MessageText {
		t.Errorf("message type = %v, want %v", msgType, websocket.MessageText)
	}
	if string(got) != want {
		t.Errorf("client received %q, want %q", got, want)
	}
}

func TestConnection_ReadLoop_ReturnsOnClientClose(t *testing.T) {
	loopReturned := make(chan struct{})
	handler := ws.NewHandler(testLogger(), testPingConfig(), func(c *ws.Connection) {
		c.ReadLoop(context.Background(), func(data []byte) {}, func() {})
		close(loopReturned)
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	clientConn, _, err := websocket.Dial(ctx, wsURL(server.URL), nil)
	if err != nil {
		t.Fatalf("client dial failed: %v", err)
	}

	if err := clientConn.Close(websocket.StatusNormalClosure, "done"); err != nil {
		t.Fatalf("client close failed: %v", err)
	}

	select {
	case <-loopReturned:
	case <-ctx.Done():
		t.Fatal("timed out waiting for ReadLoop to return after client close")
	}
}

func TestConnection_ReadOne_ReturnsFirstMessage(t *testing.T) {
	serverConnReady := make(chan *ws.Connection, 1)
	handler := ws.NewHandler(testLogger(), testPingConfig(), func(c *ws.Connection) {
		serverConnReady <- c
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

	want := `{"action":"create_room"}`
	if err := clientConn.Write(ctx, websocket.MessageText, []byte(want)); err != nil {
		t.Fatalf("client write failed: %v", err)
	}

	var serverConn *ws.Connection
	select {
	case serverConn = <-serverConnReady:
	case <-ctx.Done():
		t.Fatal("timed out waiting for server connection")
	}

	got, err := serverConn.ReadOne(ctx)
	if err != nil {
		t.Fatalf("ReadOne returned error: %v", err)
	}
	if string(got) != want {
		t.Errorf("ReadOne = %q, want %q", got, want)
	}
}

func TestConnection_ReadOne_DoesNotConsumeMessagesBeyondTheFirst(t *testing.T) {
	serverConnReady := make(chan *ws.Connection, 1)
	handler := ws.NewHandler(testLogger(), testPingConfig(), func(c *ws.Connection) {
		serverConnReady <- c
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

	if err := clientConn.Write(ctx, websocket.MessageText, []byte("first")); err != nil {
		t.Fatalf("client write failed: %v", err)
	}
	if err := clientConn.Write(ctx, websocket.MessageText, []byte("second")); err != nil {
		t.Fatalf("client write failed: %v", err)
	}

	var serverConn *ws.Connection
	select {
	case serverConn = <-serverConnReady:
	case <-ctx.Done():
		t.Fatal("timed out waiting for server connection")
	}

	first, err := serverConn.ReadOne(ctx)
	if err != nil {
		t.Fatalf("first ReadOne returned error: %v", err)
	}
	if string(first) != "first" {
		t.Fatalf("first ReadOne = %q, want %q", first, "first")
	}

	received := make(chan []byte, 1)
	go serverConn.ReadLoop(ctx, func(data []byte) {
		received <- data
	}, func() {})

	select {
	case second := <-received:
		if string(second) != "second" {
			t.Errorf("ReadLoop after ReadOne got %q, want %q", second, "second")
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for the second message via ReadLoop")
	}
}

func TestConnection_ReadOne_ReturnsErrorOnClientClose(t *testing.T) {
	resultReady := make(chan error, 1)
	handler := ws.NewHandler(testLogger(), testPingConfig(), func(c *ws.Connection) {
		_, err := c.ReadOne(context.Background())
		resultReady <- err
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	clientConn, _, err := websocket.Dial(ctx, wsURL(server.URL), nil)
	if err != nil {
		t.Fatalf("client dial failed: %v", err)
	}

	if err := clientConn.Close(websocket.StatusNormalClosure, "done"); err != nil {
		t.Fatalf("client close failed: %v", err)
	}

	select {
	case err := <-resultReady:
		if err == nil {
			t.Error("expected ReadOne to return an error after client close, got nil")
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for ReadOne to return after client close")
	}
}

func TestConnection_ReadOne_ReturnsErrorOnContextCancel(t *testing.T) {
	resultReady := make(chan error, 1)
	handler := ws.NewHandler(testLogger(), testPingConfig(), func(c *ws.Connection) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := c.ReadOne(ctx)
		resultReady <- err
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	dialCtx, dialCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer dialCancel()

	clientConn, _, err := websocket.Dial(dialCtx, wsURL(server.URL), nil)
	if err != nil {
		t.Fatalf("client dial failed: %v", err)
	}
	defer clientConn.Close(websocket.StatusNormalClosure, "")

	select {
	case err := <-resultReady:
		if err == nil {
			t.Error("expected ReadOne to return an error after context cancel, got nil")
		}
	case <-dialCtx.Done():
		t.Fatal("timed out waiting for ReadOne to return after context cancel")
	}
}

func TestConnection_CloseNow_ReturnsWithoutWaitingForPeerAck(t *testing.T) {
	closeReturned := make(chan struct{})
	handler := ws.NewHandler(testLogger(), testPingConfig(), func(c *ws.Connection) {
		c.CloseNow()
		close(closeReturned)
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

	// Deliberately do not close or read from the client connection here —
	// CloseNow must return without waiting for any cooperation from the
	// peer. coder/websocket's Close (the graceful variant) waits up to 5s
	// for the peer's close frame; this test's 2s timeout is well under
	// that, so it would fail if CloseNow ever regressed to that behavior.
	select {
	case <-closeReturned:
	case <-ctx.Done():
		t.Fatal("CloseNow did not return within 2s — it may be waiting for a close handshake like Close does")
	}
}

func TestConnection_ReadLoop_ReturnsOnContextCancel(t *testing.T) {
	loopReturned := make(chan struct{})
	handler := ws.NewHandler(testLogger(), testPingConfig(), func(c *ws.Connection) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		c.ReadLoop(ctx, func(data []byte) {}, func() {})
		close(loopReturned)
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	dialCtx, dialCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer dialCancel()

	clientConn, _, err := websocket.Dial(dialCtx, wsURL(server.URL), nil)
	if err != nil {
		t.Fatalf("client dial failed: %v", err)
	}
	defer clientConn.Close(websocket.StatusNormalClosure, "")

	select {
	case <-loopReturned:
	case <-dialCtx.Done():
		t.Fatal("timed out waiting for ReadLoop to return after context cancel")
	}
}

func TestConnection_ReadLoop_CallsOnCloseAfterClientClose(t *testing.T) {
	onCloseCalled := make(chan struct{})
	handler := ws.NewHandler(testLogger(), testPingConfig(), func(c *ws.Connection) {
		c.ReadLoop(context.Background(), func(data []byte) {}, func() {
			close(onCloseCalled)
		})
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	clientConn, _, err := websocket.Dial(ctx, wsURL(server.URL), nil)
	if err != nil {
		t.Fatalf("client dial failed: %v", err)
	}

	if err := clientConn.Close(websocket.StatusNormalClosure, "done"); err != nil {
		t.Fatalf("client close failed: %v", err)
	}

	select {
	case <-onCloseCalled:
	case <-ctx.Done():
		t.Fatal("timed out waiting for onClose to be called after client close")
	}
}

func TestConnection_ReadLoop_CallsOnCloseAfterContextCancel(t *testing.T) {
	onCloseCalled := make(chan struct{})
	handler := ws.NewHandler(testLogger(), testPingConfig(), func(c *ws.Connection) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		c.ReadLoop(ctx, func(data []byte) {}, func() {
			close(onCloseCalled)
		})
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	dialCtx, dialCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer dialCancel()

	clientConn, _, err := websocket.Dial(dialCtx, wsURL(server.URL), nil)
	if err != nil {
		t.Fatalf("client dial failed: %v", err)
	}
	defer clientConn.Close(websocket.StatusNormalClosure, "")

	select {
	case <-onCloseCalled:
	case <-dialCtx.Done():
		t.Fatal("timed out waiting for onClose to be called after context cancel")
	}
}

func TestConnection_ReadLoop_SurvivesPingsWhilePeerIsResponsive(t *testing.T) {
	loopReturned := make(chan struct{})
	pingConfig := ws.PingConfig{Interval: 50 * time.Millisecond, Timeout: 200 * time.Millisecond}
	handler := ws.NewHandler(testLogger(), pingConfig, func(c *ws.Connection) {
		c.ReadLoop(context.Background(), func(data []byte) {}, func() {})
		close(loopReturned)
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

	// A responsive client must keep reading so its WebSocket library can
	// observe and auto-reply to pings; a client that never reads would
	// never see the ping frame at all (see the unresponsive-peer test).
	go func() {
		for {
			if _, _, err := clientConn.Read(ctx); err != nil {
				return
			}
		}
	}()

	select {
	case <-loopReturned:
		t.Fatal("ReadLoop returned even though the peer was responding to pings")
	case <-time.After(300 * time.Millisecond):
		// several ping intervals have passed with no timeout — as expected
	}

	if err := clientConn.Close(websocket.StatusNormalClosure, "done"); err != nil {
		t.Fatalf("client close failed: %v", err)
	}

	select {
	case <-loopReturned:
	case <-ctx.Done():
		t.Fatal("timed out waiting for ReadLoop to return after client close")
	}
}

func TestConnection_ReadLoop_ClosesConnectionWhenPeerStopsReading(t *testing.T) {
	onCloseCalled := make(chan struct{})
	pingConfig := ws.PingConfig{Interval: 50 * time.Millisecond, Timeout: 100 * time.Millisecond}
	handler := ws.NewHandler(testLogger(), pingConfig, func(c *ws.Connection) {
		c.ReadLoop(context.Background(), func(data []byte) {}, func() {
			close(onCloseCalled)
		})
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	clientConn, _, err := websocket.Dial(ctx, wsURL(server.URL), nil)
	if err != nil {
		t.Fatalf("client dial failed: %v", err)
	}
	defer clientConn.CloseNow()

	// Deliberately never call clientConn.Read here: coder/websocket only
	// auto-replies to a ping from inside an in-progress Read, so a client
	// that never reads is indistinguishable, from the server's side, from
	// a peer that has gone silent over a partitioned network. No close
	// frame is sent either, so the only thing that can end this loop is
	// the ping timeout.
	select {
	case <-onCloseCalled:
	case <-ctx.Done():
		t.Fatal("timed out waiting for onClose to be called after the peer stopped reading")
	}
}
