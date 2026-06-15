// Package lint wraps the buf CLI to enforce proto style rules against a
// directory of .proto files. It is called during upload when lint checking
// is enabled in the server configuration.
package lint

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const bufYAML = `version: v2
lint:
  use:
    - DEFAULT
`

// Linter runs buf lint against a directory of .proto files.
type Linter struct {
	bufBin string
}

// New returns a Linter. If bufBin is empty, "buf" is used.
func New(bufBin string) *Linter {
	if bufBin == "" {
		bufBin = "buf"
	}
	return &Linter{bufBin: bufBin}
}

// Lint writes a minimal buf.yaml to protoDir and runs buf lint.
// Returns a non-nil error containing the full buf output on failure.
func (l *Linter) Lint(ctx context.Context, protoDir string) error {
	if err := writeBufYAML(protoDir); err != nil {
		return fmt.Errorf("lint: failed to write buf.yaml: %w", err)
	}
	out, err := exec.CommandContext(ctx, l.bufBin, "lint", protoDir).CombinedOutput()
	if err != nil {
		return fmt.Errorf("lint failed:\n%s", out)
	}
	return nil
}

func writeBufYAML(dir string) error {
	return os.WriteFile(filepath.Join(dir, "buf.yaml"), []byte(bufYAML), 0o644)
}
