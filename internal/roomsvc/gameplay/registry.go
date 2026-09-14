package gameplay

import "github.com/cabo/cabo-backend/internal/roomsvc/gameplay/actions"

// actionRegistry maps a wire "action" name to the ActionSpec that handles
// it. Built once, read-only after — the action set is fixed at compile
// time, not extended at runtime, so this is a plain map literal rather
// than a type with a Register method.
//
// Each key is derived from the spec's own Name(), not duplicated as a
// separate string literal — this makes a key/Name() mismatch impossible
// rather than merely tested for (see registry_test.go, which still checks
// it as a safety net against a future entry that forgets this pattern).
var actionRegistry = newActionRegistry(
	actions.DrawCardAction{},
	actions.ViewInitialCardsAction{},
)

func newActionRegistry(specs ...actions.ActionSpec) map[string]actions.ActionSpec {
	registry := make(map[string]actions.ActionSpec, len(specs))
	for _, spec := range specs {
		registry[spec.Name()] = spec
	}
	return registry
}
