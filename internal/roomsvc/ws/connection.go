// Package ws provides the WebSocket transport for roomsvc: accepting client
// connections and moving raw frames in and out. It has no knowledge of game
// rules or room state — callers plug that in via the hooks on Handler and
// Connection.
package ws

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// PingConfig controls the ping/pong keepalive ReadLoop runs for the life of
// the connection, to detect a peer that has gone silent without a normal
// WebSocket close (e.g. a network partition) — a plain read error can't
// detect this, since a dead-but-silent TCP connection simply blocks Read
// forever instead of returning an error.
type PingConfig struct {
	// Interval is how often a ping is sent while the connection is open.
	Interval time.Duration
	// Timeout is how long to wait for a pong before treating the peer as
	// unresponsive and force-closing the connection.
	Timeout time.Duration
}

// Connection wraps one accepted WebSocket connection for a single client.
type Connection struct {
	conn       *websocket.Conn
	remoteAddr string
	log        *slog.Logger
	pingConfig PingConfig

	// writeMu serializes writes: the underlying connection does not allow
	// concurrent writes from multiple goroutines.
	writeMu sync.Mutex
}

// NewConnection wraps an already-accepted WebSocket connection.
func NewConnection(conn *websocket.Conn, remoteAddr string, log *slog.Logger, pingConfig PingConfig) *Connection {
	return &Connection{
		conn:       conn,
		remoteAddr: remoteAddr,
		log:        log.With("remote_addr", remoteAddr),
		pingConfig: pingConfig,
	}
}

// RemoteAddr returns the client's network address, for logging and
// diagnostics.
func (c *Connection) RemoteAddr() string {
	return c.remoteAddr
}

// ReadOne reads exactly one message from the client and returns its
// payload. It applies the same context-cancellation handling as ReadLoop
// (see its doc comment for why). Unlike ReadLoop, ReadOne does not close
// the connection when it returns successfully — the caller decides what
// happens next, e.g. inspecting the message before deciding whether to
// continue with ReadLoop for the rest of the connection's lifetime. On
// error, the connection has already been closed, same as ReadLoop.
func (c *Connection) ReadOne(ctx context.Context) ([]byte, error) {
	stopWatchingCtx := context.AfterFunc(ctx, func() {
		c.Close(websocket.StatusNormalClosure, "server shutting down")
	})
	defer stopWatchingCtx()

	return c.readFrame(ctx)
}

// ReadLoop reads frames from the client until the connection closes or ctx
// is cancelled, calling onMessage with each frame's payload. onMessage is
// the seam where message-protocol parsing and room/game routing will plug
// in once that contract is decided.
//
// onClose runs exactly once, after the loop has ended for any reason
// (client close, read error, ctx cancellation, or a ping timeout — see
// below) — the seam for a caller to release whatever it associated with
// this connection while the loop was running (e.g. seating a player in a
// room), without ws needing any knowledge of what that association is.
//
// ReadLoop closes the connection before returning, so callers do not need
// to call Close separately in the normal case.
//
// coder/websocket's Read does not reliably unblock as soon as ctx is
// cancelled — it can take several seconds. ReadLoop instead watches ctx
// itself and force-closes the connection when it's done, since closing the
// underlying connection is what actually interrupts a blocked Read
// immediately.
//
// ReadLoop also runs a ping/pong keepalive for the life of the loop (see
// pingLoop), to detect a peer that has gone silent without a normal
// WebSocket close — e.g. a network partition, where the underlying TCP
// connection blocks Read forever instead of erroring. A ping timeout closes
// the connection the same way a read error does, so it unblocks Read and
// this loop returns through the same path.
func (c *Connection) ReadLoop(ctx context.Context, onMessage func(data []byte), onClose func()) {
	c.log.Info("connection opened")
	defer c.log.Info("connection closed")
	defer onClose()

	stopWatchingCtx := context.AfterFunc(ctx, func() {
		c.Close(websocket.StatusNormalClosure, "server shutting down")
	})
	defer stopWatchingCtx()

	pingCtx, stopPinging := context.WithCancel(ctx)
	defer stopPinging()
	go c.pingLoop(pingCtx)

	for {
		data, err := c.readFrame(ctx)
		if err != nil {
			return
		}
		onMessage(data)
	}
}

// pingLoop sends a ping every c.pingConfig.Interval for as long as ctx is
// not done, and force-closes the connection if any single ping does not
// get a pong back within c.pingConfig.Timeout. This is what detects a
// silently dead connection (e.g. a network partition) that a plain Read
// would otherwise block on forever.
//
// coder/websocket's Ping only completes once the peer's pong is observed
// by an in-progress Read on this connection, so pingLoop must run
// concurrently with ReadLoop's read loop, never on its own.
func (c *Connection) pingLoop(ctx context.Context) {
	ticker := time.NewTicker(c.pingConfig.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pingCtx, cancel := context.WithTimeout(ctx, c.pingConfig.Timeout)
			err := c.conn.Ping(pingCtx)
			cancel()
			if err != nil {
				if ctx.Err() != nil {
					// ctx ended (ReadLoop returned) while this ping was in
					// flight — not an unresponsive peer, nothing to do.
					return
				}
				c.log.Warn("ping timed out, closing unresponsive connection", "error", err)
				c.CloseNow()
				return
			}
		}
	}
}

// readFrame reads a single frame from the underlying connection,
// classifying and logging any error and closing the connection on
// failure. Shared by ReadOne and ReadLoop so that handling stays in one
// place; each caller is responsible for its own ctx-cancellation watcher,
// since ReadOne needs one scoped to a single read and ReadLoop needs one
// scoped to the whole loop.
func (c *Connection) readFrame(ctx context.Context) ([]byte, error) {
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
			// it is done because in other 2 cases connection was already closed by conn / cx
			c.Close(websocket.StatusInternalError, "read failed")
		}
		return nil, err
	}

	c.log.Debug("message received", "bytes", len(data))
	return data, nil
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
// reason, performing a full WebSocket close handshake: it waits up to 5s
// for the peer to send back its own close frame before returning. Safe to
// call multiple times. Prefer CloseNow when the peer has no reason to
// cooperate (e.g. it was just told why the connection is ending through
// some other channel already, or the connection is already broken).
func (c *Connection) Close(code websocket.StatusCode, reason string) {
	if err := c.conn.Close(code, reason); err != nil {
		c.log.Debug("close error (connection likely already closed)", "error", err)
	}
}

// CloseNow closes the connection immediately, without attempting a
// WebSocket close handshake or waiting for the peer to acknowledge. Use
// this instead of Close when there is no cooperative goodbye to wait for
// — e.g. after already telling the client why via an application-level
// message, since coder/websocket's Close otherwise blocks for up to 5s
// waiting for a close frame the peer has no reason to send.
func (c *Connection) CloseNow() {
	if err := c.conn.CloseNow(); err != nil {
		c.log.Debug("close error (connection likely already closed)", "error", err)
	}
}
