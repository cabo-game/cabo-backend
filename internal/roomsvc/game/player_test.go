package game

import "testing"

func TestNewPlayer_SetsConnection(t *testing.T) {
	player := NewPlayer(nil)
	if player.Conn != nil {
		t.Errorf("Conn = %v, want nil (passed through as given)", player.Conn)
	}
}

func TestNewPlayer_GeneratesNonEmptyID(t *testing.T) {
	player := NewPlayer(nil)
	if player.ID == "" {
		t.Error("ID is empty, want a generated player id")
	}
}

func TestNewPlayer_GeneratesDifferentIDsAcrossCalls(t *testing.T) {
	first := NewPlayer(nil)
	second := NewPlayer(nil)

	if first.ID == second.ID {
		t.Errorf("two consecutive calls produced the same id %q; expected different ids", first.ID)
	}
}
