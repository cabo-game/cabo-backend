package game

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/cabo/cabo-backend/internal/roomsvc/cards"
	"github.com/cabo/cabo-backend/internal/roomsvc/ws"
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

	// mu guards Players and started so concurrent JoinRoom calls cannot
	// both pass the capacity check and overfill the room, and so
	// StartGame cannot deal twice.
	mu      sync.Mutex
	started bool
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
//
// isFull reports whether this join filled the room's last seat, computed
// under the same lock as the seat assignment so it can't go stale between
// the join and the caller checking it. Callers use this to decide whether
// to call StartGame (see cabo-bmad/docs/cabo.md, "Minimum players to
// start: 4 (same as max)").
func (r *GameRoom) JoinRoom(player *Player) (isFull bool, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.Players) >= r.MaxPlayers {
		r.log.Info("join rejected: room full", "room_id", r.ID, "player_id", player.ID, "max_players", r.MaxPlayers)
		return false, fmt.Errorf("roomsvc: room %s is full (max %d players)", r.ID, r.MaxPlayers)
	}

	r.Players = append(r.Players, player)
	r.log.Info("player joined room", "room_id", r.ID, "player_id", player.ID, "player_count", len(r.Players))

	return len(r.Players) >= r.MaxPlayers, nil
}

// IsFull reports whether the room currently has no free seats. Safe for
// concurrent use. Unlike JoinRoom's returned isFull, this is a snapshot
// taken independently of any particular join — it exists for callers that
// need to check fullness right after room creation, before any JoinRoom
// call happens (a room created with maxPlayers=1 is already full with
// just its first player).
func (r *GameRoom) IsFull() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	return len(r.Players) >= r.MaxPlayers
}

// StartGame deals CardsPerPlayerAtStart-sized hands to every seated
// player from a freshly shuffled standard deck, and stores whatever cards
// are left over as the room's draw pile (State.RemainingCards). Callers
// trigger this once the room is full (see cabo-bmad/docs/cabo.md,
// "Minimum players to start").
//
// StartGame can only run once per room — Cabo has no re-dealing mid-game
// — so a second call returns an error instead of dealing again.
func (r *GameRoom) StartGame(cardsPerPlayer int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.started {
		r.log.Warn("start game rejected: already started", "room_id", r.ID)
		return fmt.Errorf("roomsvc: room %s has already started", r.ID)
	}

	deck := cards.NewDeck()
	cards.Shuffle(deck)

	hands, remaining, err := cards.Deal(deck, len(r.Players), cardsPerPlayer)
	if err != nil {
		r.log.Error("start game failed to deal cards", "room_id", r.ID, "player_count", len(r.Players), "cards_per_player", cardsPerPlayer, "error", err)
		return fmt.Errorf("roomsvc: room %s: %w", r.ID, err)
	}

	turnOrder := make([]string, len(r.Players))
	for i, player := range r.Players {
		player.Hand = hands[i]
		turnOrder[i] = player.ID
	}
	r.State.RemainingCards = remaining
	r.State.TurnOrder = turnOrder
	r.started = true

	r.log.Info("game started", "room_id", r.ID, "player_count", len(r.Players), "cards_per_player", cardsPerPlayer, "remaining_cards", len(remaining))

	return nil
}

// CurrentPlayerID returns the ID of the player whose turn it currently is.
// Safe for concurrent use.
func (r *GameRoom) CurrentPlayerID() string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.State.CurrentPlayerID()
}

// AdvanceTurn moves the turn to the next player in seat order, wrapping
// after the last player. Safe for concurrent use.
func (r *GameRoom) AdvanceTurn() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.State.AdvanceTurn()
}

// SeatedPlayerIDs returns the IDs of every player currently seated, in
// seat order. Safe for concurrent use. Read-only — unlike Players, it
// does not expose *Player (and therefore not *ws.Connection either),
// which is what makes it safe to hand to gameplay action handlers via
// ActionView.
func (r *GameRoom) SeatedPlayerIDs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	ids := make([]string, len(r.Players))
	for i, p := range r.Players {
		ids[i] = p.ID
	}
	return ids
}

// PlayerConn returns the *ws.Connection of the seated player with the
// given ID, and whether one was found. Safe for concurrent use.
//
// This is deliberately narrower than exposing the *Player itself: package
// gameplay's Dispatcher is the only caller (it needs a connection to
// deliver a PlayerView to), and handing back the full *Player would also
// expose Hand, which is not what delivery needs and is exactly the kind
// of unrestricted access ActionView (see action_view.go) exists to avoid
// for action handlers. PlayerConn does not implement ActionView — it is
// for the dispatcher's delivery step, not for action logic.
func (r *GameRoom) PlayerConn(playerID string) (*ws.Connection, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	player := r.findPlayerLocked(playerID)
	if player == nil {
		return nil, false
	}
	return player.Conn, true
}

// findPlayerLocked returns the player with the given ID, or nil if no
// seated player matches. Callers must hold r.mu.
func (r *GameRoom) findPlayerLocked(playerID string) *Player {
	for _, p := range r.Players {
		if p.ID == playerID {
			return p
		}
	}
	return nil
}

// DrawTopCard pops the top card of the draw pile and records it as
// playerID's pending DrawnCard. It does not add the card to Hand — Cabo
// only changes hand size once a drawn card is later resolved (swapped in
// or discarded), not on the draw itself; that resolution is not built yet.
//
// Errors if playerID is not seated, already has a pending DrawnCard (must
// resolve it before drawing again), or the draw pile is empty.
func (r *GameRoom) DrawTopCard(playerID string) (cards.Card, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	player := r.findPlayerLocked(playerID)
	if player == nil {
		return cards.Card{}, fmt.Errorf("roomsvc: player %s is not seated in room %s", playerID, r.ID)
	}
	if player.DrawnCard != nil {
		return cards.Card{}, fmt.Errorf("roomsvc: player %s already has an unresolved drawn card", playerID)
	}
	if len(r.State.RemainingCards) == 0 {
		return cards.Card{}, fmt.Errorf("roomsvc: room %s draw pile is empty", r.ID)
	}

	drawn := r.State.RemainingCards[0]
	r.State.RemainingCards = r.State.RemainingCards[1:]
	player.DrawnCard = &drawn

	return drawn, nil
}

// HandOf returns a copy of playerID's current hand. Errors if playerID is
// not seated.
func (r *GameRoom) HandOf(playerID string) ([]cards.Card, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	player := r.findPlayerLocked(playerID)
	if player == nil {
		return nil, fmt.Errorf("roomsvc: player %s is not seated in room %s", playerID, r.ID)
	}

	hand := make([]cards.Card, len(player.Hand))
	copy(hand, player.Hand)
	return hand, nil
}

// MarkInitialCardsViewed flips playerID's ViewedInitialCards flag. Errors
// if playerID is not seated or has already viewed their initial cards —
// the peek is one-time only.
func (r *GameRoom) MarkInitialCardsViewed(playerID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	player := r.findPlayerLocked(playerID)
	if player == nil {
		return fmt.Errorf("roomsvc: player %s is not seated in room %s", playerID, r.ID)
	}
	if player.ViewedInitialCards {
		return fmt.Errorf("roomsvc: player %s has already viewed their initial cards", playerID)
	}

	player.ViewedInitialCards = true
	return nil
}

// RemovePlayer removes the player with the given ID from the room, e.g.
// when their connection closes. It reports whether the room is now empty,
// so a caller (RoomManager) can decide whether to remove the room itself.
// Safe for concurrent use, under the same lock as JoinRoom. If no player
// with playerID is seated (e.g. RemovePlayer is called twice for the same
// disconnect), it logs a warning and leaves the room unchanged rather than
// erroring — a connection that is already gone is not a failure case for
// the caller to handle.
func (r *GameRoom) RemovePlayer(playerID string) (isEmpty bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, p := range r.Players {
		if p.ID == playerID {
			r.Players = append(r.Players[:i], r.Players[i+1:]...)
			r.log.Info("player removed from room", "room_id", r.ID, "player_id", playerID, "player_count", len(r.Players))
			return len(r.Players) == 0
		}
	}

	r.log.Warn("player removal requested but player not found in room", "room_id", r.ID, "player_id", playerID)
	return len(r.Players) == 0
}
