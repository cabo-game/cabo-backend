package api

import (
	"errors"
	"net/http"

	"github.com/cabo/cabo-backend/internal/authsvc/serverpicker"
)

type allocateServerResponse struct {
	ServerAddress string `json:"server_address"`
}

// handleAllocateServer implements AD-9 step 1: a client asks for a live
// roomsvc server to create a room on. No room or room ID exists yet.
func (s *Server) handleAllocateServer(w http.ResponseWriter, r *http.Request) {
	address, err := s.serverPicker.PickServer(r.Context())
	if errors.Is(err, serverpicker.ErrNoServersAvailable) {
		writeError(w, s.log, http.StatusServiceUnavailable, "no roomsvc servers are currently available")
		return
	}
	if err != nil {
		s.log.Error("failed to pick a server", "error", err)
		writeError(w, s.log, http.StatusInternalServerError, "failed to allocate a server")
		return
	}

	writeJSON(w, s.log, http.StatusOK, allocateServerResponse{ServerAddress: address})
}
