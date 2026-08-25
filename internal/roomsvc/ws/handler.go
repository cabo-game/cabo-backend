package ws

import (
	"log/slog"
	"net/http"

	"github.com/coder/websocket"
)

// Handler upgrades incoming HTTP requests to WebSocket connections and hands
// each accepted Connection to OnConnect.
type Handler struct {
	log *slog.Logger

	// OnConnect is called once per accepted connection, in its own
	// goroutine. This is the seam where auth validation and room
	// assignment will plug in, once that handoff contract is decided
	// (see spine AD-6/AD-7 deferred items). For now, callers own reading
	// from the connection via Connection.ReadLoop.
	OnConnect func(*Connection)
}

// NewHandler builds a Handler. OnConnect must be set by the caller before
// the handler serves any requests.
func NewHandler(log *slog.Logger, onConnect func(*Connection)) *Handler {
	return &Handler{
		log:       log,
		OnConnect: onConnect,
	}
}

// ServeHTTP upgrades the request to a WebSocket connection and dispatches
// it to OnConnect. If the upgrade fails, websocket.Accept has already
// written the appropriate HTTP error response.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		h.log.Warn("websocket upgrade failed", "error", err, "remote_addr", r.RemoteAddr)
		return
	}

	connection := NewConnection(conn, r.RemoteAddr, h.log)
	go h.OnConnect(connection)
}
