// Integration tests for the breaking-change checker.
// Requires the buf CLI on PATH. buf.yaml is written by the test (in production
// it is written by runProtoChecks before Check is called).
package breaking

import (
	"context"
	"os"
	"path/filepath"
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
	if err := c.Check(context.Background(), v2Dir, v1Dir); err == nil {
		t.Fatal("expected breaking error, got nil")
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

