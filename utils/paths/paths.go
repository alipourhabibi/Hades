package paths

import (
	"slices"
	"strings"

	"github.com/alipourhabibi/Hades/models"
)

var validPathes = []string{
	"buf.md",
	"README.md",
	"README.markdown",
	"LICENSE",
}

func GetPath(files []*models.File) []*models.File {
	retFiles := []*models.File{}
	for _, f := range files {
		if strings.HasSuffix(f.Path, ".proto") || slices.Contains(validPathes, f.Path) {
			retFiles = append(retFiles, f)
		}
	}
	return retFiles
}
