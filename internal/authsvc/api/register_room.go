package api

import (
	"encoding/json"
	"net/http"
)

type registerRoomRequest struct {
	RoomID        string `json:"room_id"`
	ServerAddress string `json:"server_address"`
}

// handleRegisterRoom implements AD-9 step 3-4: roomsvc reports a room it
// just created, and authsvc records room-id -> server-address (spine
// AD-3 keeps this write on the stateless tier).
//
func (s *Server) handleRegisterRoom(w http.ResponseWriter, r *http.Request) {
	var req registerRoomRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, s.log, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.RoomID == "" || req.ServerAddress == "" {
		writeError(w, s.log, http.StatusBadRequest, "room_id and server_address are required")
		return
	}

	if err := s.roomDirectory.RegisterRoom(r.Context(), req.RoomID, req.ServerAddress); err != nil {
		s.log.Error("failed to register room", "room_id", req.RoomID, "error", err)
		writeError(w, s.log, http.StatusInternalServerError, "failed to register room")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
