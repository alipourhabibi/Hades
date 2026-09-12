package paths

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
)

func TestValidatePath(t *testing.T) {
	cases := []struct {
		name string
		path string
		ok   bool
	}{
		{"plain proto", "foo.proto", true},
		{"nested proto", "proto/foo/bar.proto", true},
		{"readme", "README.md", true},

		// The traversal cases. Each of these ends in ".proto", so a suffix
		// filter alone lets them through and writeProtoFile would escape the
		// working directory.
		{"parent traversal", "../evil.proto", false},
		{"deep traversal", "../../../../etc/cron.d/evil.proto", false},
		{"traversal in the middle", "a/../../b.proto", false},
		{"absolute", "/etc/evil.proto", false},
		{"current dir segment", "./foo.proto", false},

		{"empty", "", false},
		{"empty segment", "a//b.proto", false},
		{"backslash", `a\b.proto`, false},
		{"nul byte", "a\x00b.proto", false},
		{"trailing slash", "a/b/", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidatePath(tc.path)
			if tc.ok {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err, "path %q must be rejected", tc.path)
			}
		})
	}
}

func TestValidatePath_RejectsOverlongPath(t *testing.T) {
	long := make([]byte, maxPathLength+1)
	for i := range long {
		long[i] = 'a'
	}
	assert.Error(t, ValidatePath(string(long)))
}

func TestGetPath_DropsUnsafePaths(t *testing.T) {
	files := []*registryv1.File{
		{Path: "good.proto"},
		{Path: "../escape.proto"},
		{Path: "nested/ok.proto"},
		{Path: "/absolute.proto"},
		{Path: "README.md"},
		{Path: "notes.txt"}, // filtered for being an unlisted file type
	}

	got := GetPath(files)

	paths := make([]string, 0, len(got))
	for _, f := range got {
		paths = append(paths, f.Path)
	}
	assert.ElementsMatch(t, []string{"good.proto", "nested/ok.proto", "README.md"}, paths)
}

func TestValidate_ReportsFirstUnsafePath(t *testing.T) {
	err := Validate([]*registryv1.File{
		{Path: "fine.proto"},
		{Path: "../../evil.proto"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "evil.proto")
}

func TestValidate_AcceptsCleanSet(t *testing.T) {
	assert.NoError(t, Validate([]*registryv1.File{
		{Path: "a.proto"},
		{Path: "b/c.proto"},
	}))
}
