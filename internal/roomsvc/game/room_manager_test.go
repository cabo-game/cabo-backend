package game

import (
	"sync"
	"testing"
)

// fakeNotifier is a test double for authclient.Client, recording every
// room ID it's told about instead of making a real HTTP call.
type fakeNotifier struct {
	mu       sync.Mutex
	notified []string
}

func (f *fakeNotifier) NotifyRoomCreated(roomID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.notified = append(f.notified, roomID)
}

func (f *fakeNotifier) notifiedRooms() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.notified))
	copy(out, f.notified)
	return out
}

func TestRoomManager_CreateRoom_RegistersRoomForLookup(t *testing.T) {
	manager := NewRoomManager(testLogger(), &fakeNotifier{})

	created, err := manager.CreateRoom(&Player{ID: "player-1"}, MaxPlayersPerRoom)
	if err != nil {
		t.Fatalf("CreateRoom returned error: %v", err)
	}

	found, err := manager.GetRoom(created.ID)
	if err != nil {
		t.Fatalf("GetRoom returned error: %v", err)
	}
	if found != created {
		t.Errorf("GetRoom returned %v, want %v", found, created)
	}
}

func TestRoomManager_CreateRoom_NotifiesAuthsvcOfTheNewRoom(t *testing.T) {
	notifier := &fakeNotifier{}
	manager := NewRoomManager(testLogger(), notifier)

	created, err := manager.CreateRoom(&Player{ID: "player-1"}, MaxPlayersPerRoom)
	if err != nil {
		t.Fatalf("CreateRoom returned error: %v", err)
	}

	got := notifier.notifiedRooms()
	if len(got) != 1 || got[0] != created.ID {
		t.Errorf("notified rooms = %v, want [%q]", got, created.ID)
	}
}

func TestRoomManager_CreateRoom_PropagatesValidationError(t *testing.T) {
	manager := NewRoomManager(testLogger(), &fakeNotifier{})

	_, err := manager.CreateRoom(&Player{ID: "player-1"}, 0)
	if err == nil {
		t.Fatal("expected an error for maxPlayers = 0, got nil")
	}
}

func TestRoomManager_CreateRoom_DoesNotNotifyOnValidationError(t *testing.T) {
	notifier := &fakeNotifier{}
	manager := NewRoomManager(testLogger(), notifier)

	if _, err := manager.CreateRoom(&Player{ID: "player-1"}, 0); err == nil {
		t.Fatal("expected an error for maxPlayers = 0, got nil")
	}

	if got := notifier.notifiedRooms(); len(got) != 0 {
		t.Errorf("notified rooms = %v, want none (room was never created)", got)
	}
}

func TestRoomManager_GetRoom_ReturnsErrorForUnknownID(t *testing.T) {
	manager := NewRoomManager(testLogger(), &fakeNotifier{})

	_, err := manager.GetRoom("does-not-exist")
	if err == nil {
		t.Error("expected an error for an unknown room id, got nil")
	}
}

func TestRoomManager_CreateRoom_ConcurrentCreatesAreAllRetrievable(t *testing.T) {
	manager := NewRoomManager(testLogger(), &fakeNotifier{})

	const roomsToCreate = 20
	var wg sync.WaitGroup
	ids := make(chan string, roomsToCreate)

	for i := 0; i < roomsToCreate; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			room, err := manager.CreateRoom(&Player{ID: "player"}, MaxPlayersPerRoom)
			if err != nil {
				t.Errorf("CreateRoom returned error: %v", err)
				return
			}
			ids <- room.ID
		}(i)
	}
	wg.Wait()
	close(ids)

	seen := make(map[string]bool)
	for id := range ids {
		seen[id] = true
	}
	if len(seen) != roomsToCreate {
		t.Fatalf("got %d distinct created room ids, want %d", len(seen), roomsToCreate)
	}

	for id := range seen {
		if _, err := manager.GetRoom(id); err != nil {
			t.Errorf("GetRoom(%q) returned error: %v", id, err)
		}
	}
}
