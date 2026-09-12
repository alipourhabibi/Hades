// Package lint wraps the buf CLI to enforce proto style rules against a
// directory of .proto files. It is called during upload when lint checking
// is enabled in the server configuration.
//
// The caller (runProtoChecks) is responsible for writing buf.yaml to
// protoDir before calling Lint. This package does not create it.
package lint

import (
	"context"
	"fmt"
	"os/exec"
)

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

// Lint runs buf lint against protoDir. buf.yaml must already exist in protoDir.
func (l *Linter) Lint(ctx context.Context, protoDir string) error {
	out, err := exec.CommandContext(ctx, l.bufBin, "lint", protoDir).CombinedOutput()
	if err != nil {
		return fmt.Errorf("lint failed:\n%s", out)
	}
	return nil
}
