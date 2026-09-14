package gameplay

import (
	"testing"

	"github.com/cabo/cabo-backend/internal/roomsvc/gameplay/actions"
)

type stubSpec struct {
	requiresCurrentTurn bool
}

func (s stubSpec) Name() string                { return "stub" }
func (s stubSpec) RequiresCurrentTurn() bool    { return s.requiresCurrentTurn }
func (s stubSpec) Handle(actions.ActionContext) (actions.Effect, error) {
	return actions.Effect{}, nil
}

func TestCheckTurn_AllowsWhenActionDoesNotRequireCurrentTurn(t *testing.T) {
	err := checkTurn(stubSpec{requiresCurrentTurn: false}, "player-2", "player-1")
	if err != nil {
		t.Errorf("checkTurn returned error %v, want nil (action is not turn-gated)", err)
	}
}

func TestCheckTurn_AllowsCurrentPlayer(t *testing.T) {
	err := checkTurn(stubSpec{requiresCurrentTurn: true}, "player-1", "player-1")
	if err != nil {
		t.Errorf("checkTurn returned error %v, want nil (acting player is the current player)", err)
	}
}

func TestCheckTurn_RejectsNonCurrentPlayer(t *testing.T) {
	err := checkTurn(stubSpec{requiresCurrentTurn: true}, "player-2", "player-1")
	if err == nil {
		t.Error("checkTurn returned nil, want an error (acting player is not the current player)")
	}
}
