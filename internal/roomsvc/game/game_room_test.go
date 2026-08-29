package game

import (
	"io"
	"log/slog"
	"sync"
	"testing"
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

	if err := room.JoinRoom(&Player{ID: "player-2"}); err != nil {
		t.Fatalf("JoinRoom returned error: %v", err)
	}

	if len(room.Players) != 2 {
		t.Errorf("len(room.Players) = %d, want 2", len(room.Players))
	}
}

func TestGameRoom_JoinRoom_RejectsWhenFull(t *testing.T) {
	room, err := NewGameRoom(&Player{ID: "player-1"}, 1, testLogger())
	if err != nil {
		t.Fatalf("NewGameRoom returned error: %v", err)
	}

	err = room.JoinRoom(&Player{ID: "player-2"})
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
			if err := room.JoinRoom(&Player{ID: "concurrent-player"}); err == nil {
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
	if err := room.JoinRoom(second); err != nil {
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
