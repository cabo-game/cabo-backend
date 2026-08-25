package game

import "testing"

func TestGenerateRoomCode_HasExpectedLength(t *testing.T) {
	code := generateRoomCode()
	if len(code) != roomCodeLength {
		t.Errorf("len(code) = %d, want %d", len(code), roomCodeLength)
	}
}

func TestGenerateRoomCode_OnlyUsesAllowedAlphabet(t *testing.T) {
	allowed := make(map[rune]bool)
	for _, r := range roomCodeAlphabet {
		allowed[r] = true
	}

	code := generateRoomCode()
	for _, r := range code {
		if !allowed[r] {
			t.Errorf("code %q contains disallowed character %q", code, r)
		}
	}
}

func TestGenerateRoomCode_DiffersAcrossCalls(t *testing.T) {
	first := generateRoomCode()
	second := generateRoomCode()

	if first == second {
		t.Errorf("two consecutive calls produced the same code %q; expected different codes", first)
	}
}
