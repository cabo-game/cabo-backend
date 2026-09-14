package gameplay

import "testing"

func TestActionRegistry_LookupKnownAction(t *testing.T) {
	for _, name := range []string{"draw_card", "view_initial_cards"} {
		if _, ok := actionRegistry[name]; !ok {
			t.Errorf("actionRegistry[%q] not found, want a registered ActionSpec", name)
		}
	}
}

func TestActionRegistry_NameMatchesRegisteredKey(t *testing.T) {
	for key, spec := range actionRegistry {
		if spec.Name() != key {
			t.Errorf("actionRegistry[%q].Name() = %q, want %q", key, spec.Name(), key)
		}
	}
}

func TestActionRegistry_UnknownActionNotFound(t *testing.T) {
	if _, ok := actionRegistry["does_not_exist"]; ok {
		t.Error(`actionRegistry["does_not_exist"] found, want not present`)
	}
}
