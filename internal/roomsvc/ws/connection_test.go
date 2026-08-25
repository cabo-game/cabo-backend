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
	handler := ws.NewHandler(testLogger(), func(c *ws.Connection) {
		c.ReadLoop(context.Background(), func(data []byte) {
			received <- data
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
	handler := ws.NewHandler(testLogger(), func(c *ws.Connection) {
		serverConnReady <- c
		c.ReadLoop(context.Background(), func(data []byte) {})
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
	handler := ws.NewHandler(testLogger(), func(c *ws.Connection) {
		c.ReadLoop(context.Background(), func(data []byte) {})
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

func TestConnection_ReadLoop_ReturnsOnContextCancel(t *testing.T) {
	loopReturned := make(chan struct{})
	handler := ws.NewHandler(testLogger(), func(c *ws.Connection) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		c.ReadLoop(ctx, func(data []byte) {})
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
