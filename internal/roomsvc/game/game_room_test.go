package game

import (
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/cabo/cabo-backend/internal/roomsvc/cards"
	"github.com/cabo/cabo-backend/internal/roomsvc/ws"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// newTestRoom creates a GameRoom seated with n players (IDs "player-1",
// "player-2", ...) and returns the room alongside its players in seat
// order. Fails the test immediately on any setup error.
func newTestRoom(t *testing.T, n int) (*GameRoom, []*Player) {
	t.Helper()

	players := make([]*Player, n)
	players[0] = &Player{ID: "player-1"}

	room, err := NewGameRoom(players[0], n, testLogger())
	if err != nil {
		t.Fatalf("NewGameRoom returned error: %v", err)
	}

	for i := 1; i < n; i++ {
		players[i] = &Player{ID: fmt.Sprintf("player-%d", i+1)}
		if _, err := room.JoinRoom(players[i]); err != nil {
			t.Fatalf("JoinRoom returned error: %v", err)
		}
	}

	return room, players
}

// newStartedTestRoom is newTestRoom followed by StartGame with the
// standard opening hand size, for tests that only care about post-deal
// behavior.
func newStartedTestRoom(t *testing.T, n int) (*GameRoom, []*Player) {
	t.Helper()

	room, players := newTestRoom(t, n)
	if err := room.StartGame(CardsPerPlayerAtStart); err != nil {
		t.Fatalf("StartGame returned error: %v", err)
	}
	return room, players
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

func TestGameRoom_StartGame_SetsTurnOrderToSeatOrder(t *testing.T) {
	room, players := newStartedTestRoom(t, 2)

	wantOrder := []string{players[0].ID, players[1].ID}
	if len(room.State.TurnOrder) != len(wantOrder) {
		t.Fatalf("len(State.TurnOrder) = %d, want %d", len(room.State.TurnOrder), len(wantOrder))
	}
	for i, id := range wantOrder {
		if room.State.TurnOrder[i] != id {
			t.Errorf("State.TurnOrder[%d] = %q, want %q", i, room.State.TurnOrder[i], id)
		}
	}
	if room.CurrentPlayerID() != players[0].ID {
		t.Errorf("CurrentPlayerID() = %q, want %q (first-seated player goes first)", room.CurrentPlayerID(), players[0].ID)
	}
}

func TestGameRoom_DrawTopCard_PopsTopOfRemainingCardsIntoDrawnCard(t *testing.T) {
	room, players := newStartedTestRoom(t, 2)
	acting := players[0]

	wantCard := room.State.RemainingCards[0]
	wantRemainingAfter := len(room.State.RemainingCards) - 1
	wantHandSizeAfter := len(acting.Hand) // draw must not change hand size

	drawn, err := room.DrawTopCard(acting.ID)
	if err != nil {
		t.Fatalf("DrawTopCard returned error: %v", err)
	}

	if drawn != wantCard {
		t.Errorf("DrawTopCard returned %+v, want %+v (top of RemainingCards)", drawn, wantCard)
	}
	if len(room.State.RemainingCards) != wantRemainingAfter {
		t.Errorf("len(State.RemainingCards) after draw = %d, want %d", len(room.State.RemainingCards), wantRemainingAfter)
	}
	if len(acting.Hand) != wantHandSizeAfter {
		t.Errorf("len(acting.Hand) after draw = %d, want %d (draw must not change hand size)", len(acting.Hand), wantHandSizeAfter)
	}
	if acting.DrawnCard == nil || *acting.DrawnCard != wantCard {
		t.Errorf("acting.DrawnCard = %v, want pointer to %+v", acting.DrawnCard, wantCard)
	}
}

func TestGameRoom_DrawTopCard_ErrorsWhenPlayerAlreadyHasPendingDraw(t *testing.T) {
	room, players := newStartedTestRoom(t, 2)
	acting := players[0]

	if _, err := room.DrawTopCard(acting.ID); err != nil {
		t.Fatalf("first DrawTopCard returned error: %v", err)
	}
	remainingAfterFirst := len(room.State.RemainingCards)

	if _, err := room.DrawTopCard(acting.ID); err == nil {
		t.Error("second DrawTopCard before resolving the first returned nil, want an error")
	}
	if len(room.State.RemainingCards) != remainingAfterFirst {
		t.Errorf("len(State.RemainingCards) = %d, want %d (rejected draw must not pop another card)", len(room.State.RemainingCards), remainingAfterFirst)
	}
}

func TestGameRoom_DrawTopCard_ErrorsWhenDrawPileEmpty(t *testing.T) {
	room, players := newTestRoom(t, MaxPlayersPerRoom)
	// Deal out the entire deck as opening hands, leaving nothing to draw.
	if err := room.StartGame(52 / MaxPlayersPerRoom); err != nil {
		t.Fatalf("StartGame returned error: %v", err)
	}

	if _, err := room.DrawTopCard(players[0].ID); err == nil {
		t.Error("expected an error drawing from an empty pile, got nil")
	}
}

func TestGameRoom_DrawTopCard_ErrorsForUnknownPlayer(t *testing.T) {
	room, _ := newStartedTestRoom(t, 2)

	if _, err := room.DrawTopCard("does-not-exist"); err == nil {
		t.Error("expected an error for an unknown player ID, got nil")
	}
}

func TestGameRoom_AdvanceTurn_MovesToNextPlayerInSeatOrder(t *testing.T) {
	room, players := newStartedTestRoom(t, 2)

	room.AdvanceTurn()

	if room.CurrentPlayerID() != players[1].ID {
		t.Errorf("CurrentPlayerID() after AdvanceTurn = %q, want %q", room.CurrentPlayerID(), players[1].ID)
	}
}

func TestGameRoom_AdvanceTurn_WrapsAfterLastPlayer(t *testing.T) {
	room, players := newStartedTestRoom(t, 2)

	room.AdvanceTurn()
	room.AdvanceTurn()

	if room.CurrentPlayerID() != players[0].ID {
		t.Errorf("CurrentPlayerID() after wrapping = %q, want %q", room.CurrentPlayerID(), players[0].ID)
	}
}

func TestGameRoom_HandOf_ReturnsPlayersHand(t *testing.T) {
	room, players := newStartedTestRoom(t, 2)

	hand, err := room.HandOf(players[0].ID)
	if err != nil {
		t.Fatalf("HandOf returned error: %v", err)
	}

	if len(hand) != CardsPerPlayerAtStart {
		t.Errorf("len(hand) = %d, want %d", len(hand), CardsPerPlayerAtStart)
	}
	for i, c := range hand {
		if c != players[0].Hand[i] {
			t.Errorf("hand[%d] = %+v, want %+v", i, c, players[0].Hand[i])
		}
	}
}

func TestGameRoom_HandOf_ErrorsForUnknownPlayer(t *testing.T) {
	room, _ := newStartedTestRoom(t, 2)

	if _, err := room.HandOf("does-not-exist"); err == nil {
		t.Error("expected an error for an unknown player ID, got nil")
	}
}

func TestGameRoom_MarkInitialCardsViewed_SetsFlagOnFirstCall(t *testing.T) {
	room, players := newStartedTestRoom(t, 2)

	if err := room.MarkInitialCardsViewed(players[0].ID); err != nil {
		t.Fatalf("MarkInitialCardsViewed returned error: %v", err)
	}

	if !players[0].ViewedInitialCards {
		t.Error("players[0].ViewedInitialCards = false, want true")
	}
}

func TestGameRoom_MarkInitialCardsViewed_ErrorsOnSecondCall(t *testing.T) {
	room, players := newStartedTestRoom(t, 2)

	if err := room.MarkInitialCardsViewed(players[0].ID); err != nil {
		t.Fatalf("first MarkInitialCardsViewed call returned error: %v", err)
	}

	if err := room.MarkInitialCardsViewed(players[0].ID); err == nil {
		t.Error("second MarkInitialCardsViewed call returned nil, want an error (one-time peek)")
	}
}

func TestGameRoom_MarkInitialCardsViewed_ErrorsForUnknownPlayer(t *testing.T) {
	room, _ := newStartedTestRoom(t, 2)

	if err := room.MarkInitialCardsViewed("does-not-exist"); err == nil {
		t.Error("expected an error for an unknown player ID, got nil")
	}
}

func TestGameRoom_SeatedPlayerIDs_ReturnsAllSeatedPlayersInSeatOrder(t *testing.T) {
	room, players := newTestRoom(t, 3)

	ids := room.SeatedPlayerIDs()

	if len(ids) != len(players) {
		t.Fatalf("len(ids) = %d, want %d", len(ids), len(players))
	}
	for i, p := range players {
		if ids[i] != p.ID {
			t.Errorf("ids[%d] = %q, want %q", i, ids[i], p.ID)
		}
	}
}

func TestGameRoom_SeatedPlayerIDs_ReflectsRemovals(t *testing.T) {
	room, players := newTestRoom(t, 2)

	room.RemovePlayer(players[0].ID)

	ids := room.SeatedPlayerIDs()
	if len(ids) != 1 || ids[0] != players[1].ID {
		t.Errorf("SeatedPlayerIDs() = %v, want [%q]", ids, players[1].ID)
	}
}

func TestGameRoom_PlayerConn_ReturnsSeatedPlayersConnection(t *testing.T) {
	conn := &ws.Connection{}
	first := &Player{ID: "player-1", Conn: conn}
	room, err := NewGameRoom(first, MaxPlayersPerRoom, testLogger())
	if err != nil {
		t.Fatalf("NewGameRoom returned error: %v", err)
	}

	got, ok := room.PlayerConn(first.ID)
	if !ok {
		t.Fatal("PlayerConn returned ok = false, want true")
	}
	if got != conn {
		t.Errorf("PlayerConn returned %v, want %v", got, conn)
	}
}

func TestGameRoom_PlayerConn_ReportsNotFoundForUnknownPlayer(t *testing.T) {
	room, _ := newTestRoom(t, 1)

	_, ok := room.PlayerConn("does-not-exist")
	if ok {
		t.Error("PlayerConn returned ok = true for an unseated player, want false")
	}
}

func TestGameRoom_PlayerConn_ReflectsRemovals(t *testing.T) {
	room, players := newTestRoom(t, 2)

	room.RemovePlayer(players[0].ID)

	_, ok := room.PlayerConn(players[0].ID)
	if ok {
		t.Error("PlayerConn returned ok = true for a removed player, want false")
	}
}
