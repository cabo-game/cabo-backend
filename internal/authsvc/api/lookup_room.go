package api

import (
	"errors"
	"net/http"

	"github.com/cabo/cabo-backend/internal/authsvc/roomdirectory"
)

type lookupRoomResponse struct {
	ServerAddress string `json:"server_address"`
}

// handleLookupRoom implements AD-9's second-player join: a client asks for
// the server owning an existing room, by ID.
func (s *Server) handleLookupRoom(w http.ResponseWriter, r *http.Request) {
	roomID := r.PathValue("roomID")

	address, err := s.roomDirectory.LookupRoom(r.Context(), roomID)
	if errors.Is(err, roomdirectory.ErrRoomNotFound) {
		writeError(w, s.log, http.StatusNotFound, "room not found")
		return
	}
	if err != nil {
		s.log.Error("failed to look up room", "room_id", roomID, "error", err)
		writeError(w, s.log, http.StatusInternalServerError, "failed to look up room")
		return
	}

	writeJSON(w, s.log, http.StatusOK, lookupRoomResponse{ServerAddress: address})
}
