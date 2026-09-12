package git

import "testing"

// TestValidateRepoPath is the storage-layer half of the path-traversal
// defence. It is deliberately redundant with the handler's name validation:
// gogit resolves a repository with filepath.Join, which cleans ".." instead of
// refusing it, so a gap upstream must not be able to reach the filesystem.
func TestValidateRepoPath(t *testing.T) {
	rejected := []string{
		"",
		"..",
		"owner/../escape",
		"../escape",
		"owner/module/extra",
		"owner\\module",
		".git",
		"owner/.git",
		"-dash/module",
		"owner/\x00",
	}
	for _, p := range rejected {
		if err := ValidateRepoPath(p); err == nil {
			t.Errorf("ValidateRepoPath(%q) = nil, want an error", p)
		}
	}

	accepted := []string{
		"alice/mymod",
		"alice/my-mod",
		"sdk-artifacts", // the shared SDK artifact repository
	}
	for _, p := range accepted {
		if err := ValidateRepoPath(p); err != nil {
			t.Errorf("ValidateRepoPath(%q) = %v, want nil", p, err)
		}
	}
}
