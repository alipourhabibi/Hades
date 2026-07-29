// Package breaking wraps the buf CLI to detect backward-incompatible changes
// between two directories of .proto files. It is called during upload to
// reject pushes that would break existing consumers.
package breaking

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const bufYAML = `version: v2
breaking:
  use:
    - FILE
`

// Checker runs buf breaking against two directories of .proto files.
type Checker struct {
	bufBin string
}

// New returns a Checker. If bufBin is empty, "buf" is used.
func New(bufBin string) *Checker {
	if bufBin == "" {
		bufBin = "buf"
	}
	return &Checker{bufBin: bufBin}
}

// Check compares newDir against prevDir for backward-incompatible changes.
// If prevDir is empty the check is skipped (first push).
func (c *Checker) Check(ctx context.Context, newDir, prevDir string) error {
	if prevDir == "" {
		return nil
	}
	if err := writeBufYAML(newDir); err != nil {
		return fmt.Errorf("breaking: failed to write buf.yaml: %w", err)
	}
	out, err := exec.CommandContext(ctx, c.bufBin,
		"breaking", newDir, "--against", prevDir).CombinedOutput()
	if err != nil {
		return fmt.Errorf("breaking change detected:\n%s", out)
	}
	return nil
}

// writeBufYAML writes the default buf.yaml only when the directory does not
// already contain one (i.e. the module did not upload its own buf.yaml).
func writeBufYAML(dir string) error {
	path := filepath.Join(dir, "buf.yaml")
	if _, err := os.Stat(path); err == nil {
		return nil // module has its own buf.yaml; honour it
	}
	return os.WriteFile(path, []byte(bufYAML), 0o644)
}
