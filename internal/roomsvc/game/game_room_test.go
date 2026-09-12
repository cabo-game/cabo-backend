package game

import (
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/cabo/cabo-backend/internal/roomsvc/cards"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestNewGameRoom_SeatsFirstPlayer(t *testing.T) {
	firstPlayer := &Player{ID: "player-1"}

	room, err := NewGameRoom(firstPlayer, MaxPlayersPerRoom, testLogger())
	if err != nil {
		t.Fatalf("NewGameRoom returned error: %v", err)
	}

	if len(room.Players) != 1 {
		t.Fatalf("len(room.Players) = %d, want 1", len(room.Players))
	}
	if room.Players[0] != firstPlayer {
		t.Errorf("room.Players[0] = %v, want %v", room.Players[0], firstPlayer)
	}
}

func TestNewGameRoom_GeneratesNonEmptyID(t *testing.T) {
	room, err := NewGameRoom(&Player{ID: "player-1"}, MaxPlayersPerRoom, testLogger())
	if err != nil {
		t.Fatalf("NewGameRoom returned error: %v", err)
	}

	if room.ID == "" {
		t.Error("room.ID is empty, want a generated room code")
	}
}

func TestNewGameRoom_InitializesState(t *testing.T) {
	room, err := NewGameRoom(&Player{ID: "player-1"}, MaxPlayersPerRoom, testLogger())
	if err != nil {
		t.Fatalf("NewGameRoom returned error: %v", err)
	}

	if room.State == nil {
		t.Error("room.State is nil, want an initialized GameState")
	}
}

func TestNewGameRoom_SetsMaxPlayers(t *testing.T) {
	room, err := NewGameRoom(&Player{ID: "player-1"}, 2, testLogger())
	if err != nil {
		t.Fatalf("NewGameRoom returned error: %v", err)
	}

	if room.MaxPlayers != 2 {
		t.Errorf("room.MaxPlayers = %d, want 2", room.MaxPlayers)
	}
}

func TestNewGameRoom_RejectsMaxPlayersBelowOne(t *testing.T) {
	_, err := NewGameRoom(&Player{ID: "player-1"}, 0, testLogger())
	if err == nil {
		t.Error("expected an error for maxPlayers = 0, got nil")
	}
}

func TestNewGameRoom_RejectsMaxPlayersAboveGlobalLimit(t *testing.T) {
	_, err := NewGameRoom(&Player{ID: "player-1"}, MaxPlayersPerRoom+1, testLogger())
	if err == nil {
		t.Errorf("expected an error for maxPlayers = %d, got nil", MaxPlayersPerRoom+1)
	}
}

func TestNewGameRoom_AcceptsMaxPlayersAtGlobalLimit(t *testing.T) {
	_, err := NewGameRoom(&Player{ID: "player-1"}, MaxPlayersPerRoom, testLogger())
	if err != nil {
		t.Errorf("expected no error for maxPlayers = %d, got %v", MaxPlayersPerRoom, err)
	}
}

func TestGameRoom_JoinRoom_SeatsPlayerUnderCapacity(t *testing.T) {
	room, err := NewGameRoom(&Player{ID: "player-1"}, 2, testLogger())
	if err != nil {
		t.Fatalf("NewGameRoom returned error: %v", err)
	}

	isFull, err := room.JoinRoom(&Player{ID: "player-2"})
	if err != nil {
		t.Fatalf("JoinRoom returned error: %v", err)
	}
	if !isFull {
		t.Error("JoinRoom reported isFull = false, want true (room capacity is 2, now has 2)")
	}

	if len(room.Players) != 2 {
		t.Errorf("len(room.Players) = %d, want 2", len(room.Players))
	}
}

func TestGameRoom_JoinRoom_ReportsNotFullBelowCapacity(t *testing.T) {
	room, err := NewGameRoom(&Player{ID: "player-1"}, MaxPlayersPerRoom, testLogger())
	if err != nil {
		t.Fatalf("NewGameRoom returned error: %v", err)
	}

	isFull, err := room.JoinRoom(&Player{ID: "player-2"})
	if err != nil {
		t.Fatalf("JoinRoom returned error: %v", err)
	}
	if isFull {
		t.Error("JoinRoom reported isFull = true, want false (room capacity is 4, now has 2)")
	}
}

func TestGameRoom_JoinRoom_RejectsWhenFull(t *testing.T) {
	room, err := NewGameRoom(&Player{ID: "player-1"}, 1, testLogger())
	if err != nil {
		t.Fatalf("NewGameRoom returned error: %v", err)
	}

	_, err = room.JoinRoom(&Player{ID: "player-2"})
	if err == nil {
		t.Error("expected an error joining a full room, got nil")
	}
	if len(room.Players) != 1 {
		t.Errorf("len(room.Players) = %d, want 1 (join should not have mutated Players)", len(room.Players))
	}
}

func TestGameRoom_JoinRoom_ConcurrentJoinsRespectCapacity(t *testing.T) {
	room, err := NewGameRoom(&Player{ID: "player-1"}, MaxPlayersPerRoom, testLogger())
	if err != nil {
		t.Fatalf("NewGameRoom returned error: %v", err)
	}

	const attemptedJoins = 20
	var wg sync.WaitGroup
	var succeeded int32
	var mu sync.Mutex

	for i := 0; i < attemptedJoins; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := room.JoinRoom(&Player{ID: "concurrent-player"}); err == nil {
				mu.Lock()
				succeeded++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()

	const wantSucceeded = MaxPlayersPerRoom - 1 // room started with 1 seat already taken
	if int(succeeded) != wantSucceeded {
		t.Errorf("succeeded joins = %d, want %d", succeeded, wantSucceeded)
	}
	if len(room.Players) != MaxPlayersPerRoom {
		t.Errorf("len(room.Players) = %d, want %d", len(room.Players), MaxPlayersPerRoom)
	}
}

func TestGameRoom_RemovePlayer_RemovesMatchingPlayer(t *testing.T) {
	first := &Player{ID: "player-1"}
	room, err := NewGameRoom(first, MaxPlayersPerRoom, testLogger())
	if err != nil {
		t.Fatalf("NewGameRoom returned error: %v", err)
	}
	second := &Player{ID: "player-2"}
	if _, err := room.JoinRoom(second); err != nil {
		t.Fatalf("JoinRoom returned error: %v", err)
	}

	isEmpty := room.RemovePlayer(first.ID)

	if isEmpty {
		t.Error("RemovePlayer reported room empty, want not empty (player-2 still seated)")
	}
	if len(room.Players) != 1 {
		t.Fatalf("len(room.Players) = %d, want 1", len(room.Players))
	}
	if room.Players[0] != second {
		t.Errorf("room.Players[0] = %v, want %v", room.Players[0], second)
	}
}

func TestGameRoom_RemovePlayer_ReportsEmptyWhenLastPlayerLeaves(t *testing.T) {
	first := &Player{ID: "player-1"}
	room, err := NewGameRoom(first, MaxPlayersPerRoom, testLogger())
	if err != nil {
		t.Fatalf("NewGameRoom returned error: %v", err)
	}

	isEmpty := room.RemovePlayer(first.ID)

	if !isEmpty {
		t.Error("RemovePlayer reported room not empty, want empty")
	}
	if len(room.Players) != 0 {
		t.Errorf("len(room.Players) = %d, want 0", len(room.Players))
	}
}

func TestGameRoom_RemovePlayer_UnknownPlayerIDLeavesRoomUnchanged(t *testing.T) {
	first := &Player{ID: "player-1"}
	room, err := NewGameRoom(first, MaxPlayersPerRoom, testLogger())
	if err != nil {
		t.Fatalf("NewGameRoom returned error: %v", err)
	}

	isEmpty := room.RemovePlayer("does-not-exist")

	if isEmpty {
		t.Error("RemovePlayer reported room empty, want not empty (player-1 still seated)")
	}
	if len(room.Players) != 1 {
		t.Fatalf("len(room.Players) = %d, want 1 (unchanged)", len(room.Players))
	}
}

func TestGameRoom_IsFull_FalseBelowCapacity(t *testing.T) {
	room, err := NewGameRoom(&Player{ID: "player-1"}, MaxPlayersPerRoom, testLogger())
	if err != nil {
		t.Fatalf("NewGameRoom returned error: %v", err)
	}

	if room.IsFull() {
		t.Error("IsFull() = true, want false (1 of 4 seats taken)")
	}
}

func TestGameRoom_IsFull_TrueAtCapacity(t *testing.T) {
	room, err := NewGameRoom(&Player{ID: "player-1"}, 1, testLogger())
	if err != nil {
		t.Fatalf("NewGameRoom returned error: %v", err)
	}

	if !room.IsFull() {
		t.Error("IsFull() = false, want true (maxPlayers=1, already full at creation)")
	}
}

func TestGameRoom_StartGame_DealsHandsToAllPlayers(t *testing.T) {
	room, err := NewGameRoom(&Player{ID: "player-1"}, 2, testLogger())
	if err != nil {
		t.Fatalf("NewGameRoom returned error: %v", err)
	}
	if _, err := room.JoinRoom(&Player{ID: "player-2"}); err != nil {
		t.Fatalf("JoinRoom returned error: %v", err)
	}

	if err := room.StartGame(CardsPerPlayerAtStart); err != nil {
		t.Fatalf("StartGame returned error: %v", err)
	}

	for _, p := range room.Players {
		if len(p.Hand) != CardsPerPlayerAtStart {
			t.Errorf("player %s hand size = %d, want %d", p.ID, len(p.Hand), CardsPerPlayerAtStart)
		}
	}

	wantRemaining := 52 - len(room.Players)*CardsPerPlayerAtStart
	if len(room.State.RemainingCards) != wantRemaining {
		t.Errorf("len(State.RemainingCards) = %d, want %d", len(room.State.RemainingCards), wantRemaining)
	}
}

func TestGameRoom_StartGame_AllDealtCardsAreUniqueAndFromTheDeck(t *testing.T) {
	room, err := NewGameRoom(&Player{ID: "player-1"}, 2, testLogger())
	if err != nil {
		t.Fatalf("NewGameRoom returned error: %v", err)
	}
	if _, err := room.JoinRoom(&Player{ID: "player-2"}); err != nil {
		t.Fatalf("JoinRoom returned error: %v", err)
	}

	if err := room.StartGame(CardsPerPlayerAtStart); err != nil {
		t.Fatalf("StartGame returned error: %v", err)
	}

	seen := make(map[cards.Card]bool)
	for _, p := range room.Players {
		for _, c := range p.Hand {
			if seen[c] {
				t.Fatalf("card %+v dealt more than once", c)
			}
			seen[c] = true
		}
	}
	for _, c := range room.State.RemainingCards {
		if seen[c] {
			t.Fatalf("card %+v is both dealt and in the remaining pile", c)
		}
		seen[c] = true
	}
	if len(seen) != 52 {
		t.Errorf("total distinct cards accounted for = %d, want 52", len(seen))
	}
}

func TestGameRoom_StartGame_ErrorsOnSecondCall(t *testing.T) {
	room, err := NewGameRoom(&Player{ID: "player-1"}, MaxPlayersPerRoom, testLogger())
	if err != nil {
		t.Fatalf("NewGameRoom returned error: %v", err)
	}

	if err := room.StartGame(CardsPerPlayerAtStart); err != nil {
		t.Fatalf("first StartGame call returned error: %v", err)
	}

	if err := room.StartGame(CardsPerPlayerAtStart); err == nil {
		t.Error("second StartGame call returned nil, want an error (no re-dealing)")
	}
}

func TestGameRoom_StartGame_ErrorsWhenNotEnoughCardsForAllHands(t *testing.T) {
	room, err := NewGameRoom(&Player{ID: "player-1"}, MaxPlayersPerRoom, testLogger())
	if err != nil {
		t.Fatalf("NewGameRoom returned error: %v", err)
	}
	for i := 0; i < MaxPlayersPerRoom-1; i++ {
		if _, err := room.JoinRoom(&Player{ID: fmt.Sprintf("player-%d", i+2)}); err != nil {
			t.Fatalf("JoinRoom returned error: %v", err)
		}
	}

	// 4 players x 14 cards = 56, more than the 52-card deck.
	if err := room.StartGame(14); err == nil {
		t.Error("expected an error when the deck can't cover every hand, got nil")
	}
	for _, p := range room.Players {
		if p.Hand != nil {
			t.Errorf("player %s Hand = %v, want nil (StartGame should not have dealt anything on error)", p.ID, p.Hand)
		}
	}
}
