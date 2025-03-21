package paths

import (
	"slices"
	"strings"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
)

var validPathes = []string{
	"buf.md",
	"README.md",
	"README.markdown",
	"LICENSE",
}

func GetPath(files []*registryv1.File) []*registryv1.File {
	retFiles := []*registryv1.File{}
	for _, f := range files {
		if strings.HasSuffix(f.Path, ".proto") || slices.Contains(validPathes, f.Path) {
			retFiles = append(retFiles, f)
		}
	}
	return retFiles
}
