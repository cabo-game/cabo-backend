package gameplay

import (
	"fmt"

	"github.com/cabo/cabo-backend/internal/roomsvc/gameplay/actions"
)

// checkTurn enforces the turn-gating half of an action's rules: if spec
// requires the room's current-turn player and actingPlayerID isn't it,
// the action is rejected before Handle ever runs. Actions that are not
// turn-gated (e.g. the initial peek, or a future snap/slap reaction) skip
// this check entirely — RequiresCurrentTurn() false is enough, no branch
// needed at the call site.
func checkTurn(spec actions.ActionSpec, actingPlayerID, currentPlayerID string) error {
	if !spec.RequiresCurrentTurn() {
		return nil
	}
	if actingPlayerID != currentPlayerID {
		return fmt.Errorf("gameplay: it is not player %s's turn", actingPlayerID)
	}
	return nil
}
