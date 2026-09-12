// Integration tests for the breaking-change checker.
// Requires the buf CLI on PATH. buf.yaml is written by the test (in production
// it is written by runProtoChecks before Check is called).
package breaking

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const protoV1 = `syntax = "proto3";
package test.v1;
message Foo {
  string bar = 1;
}
`

// protoV2 removes field bar, backward-incompatible.
const protoV2 = `syntax = "proto3";
package test.v1;
message Foo {
}
`

func writeBufYAML(t *testing.T, dir string) {
	t.Helper()
	content := "version: v2\nbreaking:\n  use:\n    - FILE\n"
	if err := os.WriteFile(filepath.Join(dir, "buf.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeProto(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "foo.proto"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBreakingCheck_DetectsRemovedField(t *testing.T) {
	v1Dir := t.TempDir()
	v2Dir := t.TempDir()
	writeProto(t, v1Dir, protoV1)
	writeProto(t, v2Dir, protoV2)
	writeBufYAML(t, v1Dir)
	writeBufYAML(t, v2Dir)

	c := New("")
	err := c.Check(context.Background(), v2Dir, v1Dir)
	if err == nil {
		t.Fatal("expected breaking error, got nil")
	}
	// Any error is not enough. Without buf on PATH this test used to pass on
	// the "cannot run buf" error, which says nothing about the protos.
	if errors.Is(err, ErrUnavailable) {
		t.Fatalf("the check did not run, so this proves nothing: %v", err)
	}
}

func TestBreakingCheck_PassesIdentical(t *testing.T) {
	v1Dir := t.TempDir()
	v2Dir := t.TempDir()
	writeProto(t, v1Dir, protoV1)
	writeProto(t, v2Dir, protoV1)
	writeBufYAML(t, v1Dir)
	writeBufYAML(t, v2Dir)

	c := New("")
	if err := c.Check(context.Background(), v2Dir, v1Dir); err != nil {
		t.Fatalf("expected no breaking error, got: %v", err)
	}
}

// A missing buf binary must not look like a breaking change.
//
// Check used to return "breaking change detected" for any exec error, so a
// server without buf rejected every push and blamed the protos. CI ran without
// buf and this is the case that was hidden: the test wanting an error passed on
// the wrong error, and only the test wanting no error failed.
func TestBreakingCheck_MissingBufIsNotABreakingChange(t *testing.T) {
	dir := t.TempDir()
	writeBufYAML(t, dir)
	writeProto(t, dir, protoV1)

	prev := t.TempDir()
	writeBufYAML(t, prev)
	writeProto(t, prev, protoV1)

	c := New("buf-that-does-not-exist")
	err := c.Check(context.Background(), dir, prev)
	if err == nil {
		t.Fatal("expected an error when the buf binary is missing")
	}
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("want ErrUnavailable, got %v", err)
	}
	if strings.Contains(err.Error(), "breaking change detected") {
		t.Fatalf("a missing binary must not be reported as a breaking change: %v", err)
	}
}
