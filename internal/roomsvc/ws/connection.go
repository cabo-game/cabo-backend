// Package ws provides the WebSocket transport for roomsvc: accepting client
// connections and moving raw frames in and out. It has no knowledge of game
// rules or room state — callers plug that in via the hooks on Handler and
// Connection.
package ws

import (
	"context"
	"log/slog"
	"sync"

	"github.com/coder/websocket"
)

// Connection wraps one accepted WebSocket connection for a single client.
type Connection struct {
	conn       *websocket.Conn
	remoteAddr string
	log        *slog.Logger

	// writeMu serializes writes: the underlying connection does not allow
	// concurrent writes from multiple goroutines.
	writeMu sync.Mutex
}

// NewConnection wraps an already-accepted WebSocket connection.
func NewConnection(conn *websocket.Conn, remoteAddr string, log *slog.Logger) *Connection {
	return &Connection{
		conn:       conn,
		remoteAddr: remoteAddr,
		log:        log.With("remote_addr", remoteAddr),
	}
}

// RemoteAddr returns the client's network address, for logging and
// diagnostics.
func (c *Connection) RemoteAddr() string {
	return c.remoteAddr
}

// ReadLoop reads frames from the client until the connection closes or ctx
// is cancelled, calling onMessage with each frame's payload. onMessage is
// the seam where message-protocol parsing and room/game routing will plug
// in once that contract is decided.
//
// ReadLoop closes the connection before returning, so callers do not need
// to call Close separately in the normal case.
//
// coder/websocket's Read does not reliably unblock as soon as ctx is
// cancelled — it can take several seconds. ReadLoop instead watches ctx
// itself and force-closes the connection when it's done, since closing the
// underlying connection is what actually interrupts a blocked Read
// immediately.
func (c *Connection) ReadLoop(ctx context.Context, onMessage func(data []byte)) {
	c.log.Info("connection opened")
	defer c.log.Info("connection closed")

	stopWatchingCtx := context.AfterFunc(ctx, func() {
		c.Close(websocket.StatusNormalClosure, "server shutting down")
	})
	defer stopWatchingCtx()

	for {
		_, data, err := c.conn.Read(ctx)
		if err != nil {
			closeStatus := websocket.CloseStatus(err)
			switch {
			case closeStatus != -1:
				c.log.Info("client closed connection", "close_status", closeStatus)
			case ctx.Err() != nil:
				c.log.Info("connection closed: context done", "reason", ctx.Err())
			default:
				c.log.Warn("read failed, closing connection", "error", err)
				c.Close(websocket.StatusInternalError, "read failed")
			}
			return
		}

		c.log.Debug("message received", "bytes", len(data))
		onMessage(data)
	}
}

// Write sends data to the client as a single text-framed message. Safe for
// concurrent use.
func (c *Connection) Write(ctx context.Context, data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if err := c.conn.Write(ctx, websocket.MessageText, data); err != nil {
		c.log.Warn("write failed", "error", err)
		return err
	}

	c.log.Debug("message sent", "bytes", len(data))
	return nil
}

// Close closes the underlying connection with the given status code and
// reason. Safe to call multiple times.
func (c *Connection) Close(code websocket.StatusCode, reason string) {
	if err := c.conn.Close(code, reason); err != nil {
		c.log.Debug("close error (connection likely already closed)", "error", err)
	}
}
