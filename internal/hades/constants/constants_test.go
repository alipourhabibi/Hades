package constants

import "testing"

// TestValidateName_RejectsPathAndRouteHazards is the boundary check for names
// that become both a URL segment and a filesystem path.
//
// Every rejected case below was accepted before the check existed. "../escape"
// created a git repository one level above the owner's namespace, because
// filepath.Join cleans ".." rather than refusing it; "has space" created a
// database row with no repository behind it; "settings" collides with a real
// frontend route.
func TestValidateName_RejectsPathAndRouteHazards(t *testing.T) {
	rejected := []string{
		"",
		"../escape",
		"a/b",
		"a\\b",
		".git",
		"-dash",
		"dash-",
		".",
		"..",
		"has space",
		"UPPER",
		"tab\there",
		"trailing.",
		"settings", // reserved: a real route
		"go",       // reserved: the module proxy mount
		"this-name-is-far-too-long-to-be-a-namespace-segment",
	}
	for _, name := range rejected {
		if err := ValidateName(name); err == nil {
			t.Errorf("ValidateName(%q) = nil, want an error", name)
		}
	}

	accepted := []string{
		"a",
		"mymod",
		"my-mod",
		"my_mod",
		"my.mod",
		"fx-genable",
		"proto3",
	}
	for _, name := range accepted {
		if err := ValidateName(name); err != nil {
			t.Errorf("ValidateName(%q) = %v, want nil", name, err)
		}
	}
}
