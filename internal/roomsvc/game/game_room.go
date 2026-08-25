package game

import (
	"fmt"
	"log/slog"
	"sync"
)

// GameRoom is one game's authoritative state and the players connected to
// it. A room's live state lives in memory on exactly one server process for
// the duration of the game (spine AD-2).
type GameRoom struct {
	ID         string
	State      *GameState
	MaxPlayers int
	Players    []*Player

	log *slog.Logger

	// mu guards Players so concurrent JoinRoom calls cannot both pass the
	// capacity check and overfill the room.
	mu sync.Mutex
}

// NewGameRoom creates a new game room with firstPlayer already seated. A
// room must have at least one player from the moment it exists, so callers
// cannot construct a GameRoom without providing one.
//
// maxPlayers must be between 1 and MaxPlayersPerRoom (see rules.go); values
// outside that range return an error instead of constructing a room.
//
// The generated ID is not checked against other live rooms — that requires
// a room registry, which does not exist yet (see room_code.go).
func NewGameRoom(firstPlayer *Player, maxPlayers int, log *slog.Logger) (*GameRoom, error) {
	if maxPlayers < 1 || maxPlayers > MaxPlayersPerRoom {
		return nil, fmt.Errorf("roomsvc: maxPlayers must be between 1 and %d, got %d", MaxPlayersPerRoom, maxPlayers)
	}

	room := &GameRoom{
		ID:         generateRoomCode(),
		State:      &GameState{},
		MaxPlayers: maxPlayers,
		Players:    []*Player{firstPlayer},
		log:        log,
	}

	log.Info("game room created", "room_id", room.ID, "first_player_id", firstPlayer.ID, "max_players", maxPlayers)

	return room, nil
}

// JoinRoom seats player in the room if it is not already full. Safe for
// concurrent use: the capacity check and the seat assignment happen under
// the same lock, so two simultaneous joins cannot both succeed past
// capacity.
func (r *GameRoom) JoinRoom(player *Player) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.Players) >= r.MaxPlayers {
		r.log.Info("join rejected: room full", "room_id", r.ID, "player_id", player.ID, "max_players", r.MaxPlayers)
		return fmt.Errorf("roomsvc: room %s is full (max %d players)", r.ID, r.MaxPlayers)
	}

	r.Players = append(r.Players, player)
	r.log.Info("player joined room", "room_id", r.ID, "player_id", player.ID, "player_count", len(r.Players))

	return nil
}
