// Package api is authsvc's HTTP controller layer: the client-facing and
// roomsvc-facing endpoints implementing the AD-9 room-creation handoff.
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/cabo/cabo-backend/internal/authsvc/roomdirectory"
	"github.com/cabo/cabo-backend/internal/authsvc/serverpicker"
)

// Server holds authsvc's HTTP handlers and their dependencies. Handlers
// depend only on the RoomDirectory and ServerPicker interfaces, never on
// a concrete store or discovery mechanism, so either can be swapped
// without changing this package.
type Server struct {
	roomDirectory roomdirectory.RoomDirectory
	serverPicker  serverpicker.ServerPicker
	log           *slog.Logger
}

// NewServer creates a Server with the given dependencies.
func NewServer(roomDirectory roomdirectory.RoomDirectory, serverPicker serverpicker.ServerPicker, log *slog.Logger) *Server {
	return &Server{
		roomDirectory: roomDirectory,
		serverPicker:  serverPicker,
		log:           log,
	}
}

// Routes returns the HTTP handler mounting all of authsvc's endpoints.
func (s *Server) Routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /rooms/allocate", s.handleAllocateServer)
	mux.HandleFunc("POST /rooms/register", s.handleRegisterRoom)
	mux.HandleFunc("GET /rooms/{roomID}", s.handleLookupRoom)
	return mux
}

// writeJSON encodes v as the response body and logs (but does not react
// to) an encoding failure — by this point the status code is already
// written, so there is nothing left to do but record it.
func writeJSON(w http.ResponseWriter, log *slog.Logger, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Error("failed to encode response body", "error", err)
	}
}

type errorResponse struct {
	Error string `json:"error"`
}

func writeError(w http.ResponseWriter, log *slog.Logger, status int, message string) {
	writeJSON(w, log, status, errorResponse{Error: message})
}
