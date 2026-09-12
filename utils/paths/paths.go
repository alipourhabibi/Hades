// Package paths provides file path validation and filtering for module uploads.
package paths

import (
	"fmt"
	"path"
	"slices"
	"strings"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
)

// allowedFiles lists non-proto files that are accepted in a module upload.
var allowedFiles = []string{
	"buf.md",
	"README.md",
	"README.markdown",
	"LICENSE",
}

// maxPathLength bounds an accepted module-relative path.
const maxPathLength = 4096

// ValidatePath reports whether p is safe to use as a module-relative file path.
//
// Uploaded paths are written to disk verbatim during the lint and breaking
// checks and are handed to the git backend, so a path containing a ".."
// segment would escape the working directory and let a push write a file
// anywhere the server process can write. Suffix filtering alone does not stop
// that: "../../../etc/cron.d/x.proto" ends in ".proto".
func ValidatePath(p string) error {
	switch {
	case p == "":
		return fmt.Errorf("path is empty")
	case len(p) > maxPathLength:
		return fmt.Errorf("path exceeds %d characters", maxPathLength)
	case strings.ContainsRune(p, 0):
		return fmt.Errorf("path contains a NUL byte")
	case strings.ContainsRune(p, '\\'):
		return fmt.Errorf("path contains a backslash: %q", p)
	case strings.HasPrefix(p, "/"):
		return fmt.Errorf("path is absolute: %q", p)
	}

	for _, seg := range strings.Split(p, "/") {
		switch seg {
		case "":
			return fmt.Errorf("path has an empty segment: %q", p)
		case ".", "..":
			return fmt.Errorf("path has a %q segment: %q", seg, p)
		}
	}

	// Belt and braces: a cleaned path must be unchanged and must still be
	// relative. This catches anything the segment scan above misses.
	if cleaned := path.Clean(p); cleaned != p || strings.HasPrefix(cleaned, "..") || path.IsAbs(cleaned) {
		return fmt.Errorf("path is not a normalised relative path: %q", p)
	}
	return nil
}

// GetPath filters files to only those that belong in a module: .proto files
// and a small set of allowed metadata files.
//
// Files whose path fails ValidatePath are dropped. Use Validate to reject the
// upload outright instead of silently ignoring such a file.
func GetPath(files []*registryv1.File) []*registryv1.File {
	retFiles := []*registryv1.File{}
	for _, f := range files {
		if ValidatePath(f.Path) != nil {
			continue
		}
		if strings.HasSuffix(f.Path, ".proto") || slices.Contains(allowedFiles, f.Path) {
			retFiles = append(retFiles, f)
		}
	}
	return retFiles
}

// Validate returns an error describing the first file whose path is unsafe.
//
// Upload calls this before doing any work so a malicious path is rejected with
// a clear error rather than dropped silently. The buf.build protocol adapter
// carries buf's own wire types, which cannot be constrained with
// protovalidate, so this is the only path validation that covers both upload
// routes.
func Validate(files []*registryv1.File) error {
	for _, f := range files {
		if err := ValidatePath(f.Path); err != nil {
			return err
		}
	}
	return nil
}
