// Package breaking wraps the buf CLI to detect backward-incompatible changes
// between two directories of .proto files. It is called during upload to
// reject pushes that would break existing consumers.
//
// The caller (runProtoChecks) is responsible for writing buf.yaml to
// newDir before calling Check. This package does not create it.
package breaking

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
)

// ErrUnavailable means the check could not run, so we do not know if the
// change is breaking.
//
// This used to look the same as a real breaking change. Any error from exec
// became "breaking change detected", so a missing buf binary rejected every
// push with a message saying the protos were at fault. CI hit this: buf was
// not installed, and the test that wanted an error passed for the wrong
// reason while the test that wanted none failed.
var ErrUnavailable = errors.New("breaking: cannot run buf")

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
// If prevDir is empty the check is skipped. buf.yaml must already exist in newDir.
func (c *Checker) Check(ctx context.Context, newDir, prevDir string) error {
	if prevDir == "" {
		return nil
	}
	out, err := exec.CommandContext(ctx, c.bufBin,
		"breaking", newDir, "--against", prevDir).CombinedOutput()
	if err == nil {
		return nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("%w: %w", ErrUnavailable, ctxErr)
	}
	// buf ran and said no. That is a real breaking change.
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return fmt.Errorf("breaking change detected:\n%s", out)
	}
	// buf did not run at all.
	return fmt.Errorf("%w: %w", ErrUnavailable, err)
}
