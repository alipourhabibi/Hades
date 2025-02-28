package shake256

import (
	"bytes"
	"fmt"
	"slices"
	"strings"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
)

func DigestFiles(datas []*registryv1.File) (*digest, error) {
	digests := ""

	// Sort slice to be consistent with every order
	slices.SortFunc(datas, func(a, b *registryv1.File) int {
		if a.Path > b.Path {
			return 1
		} else if a.Path < b.Path {
			return -1
		} else {
			return 0
		}
	})

	for _, v := range datas {
		ioContent := bytes.NewReader(v.Content)
		d, err := NewDigestForContent(ioContent)
		if err != nil {
			return nil, err
		}
		digests += fmt.Sprintf("%s  %s\n", d.String(), v.Path)
	}

	// HERE TODO should look it up
	digestOfDigests, err := NewDigestForContent(strings.NewReader(digests))
	if err != nil {
		panic(err)
	}

	digestsForAllFiles, err := newDigest(digestOfDigests.Value())
	if err != nil {
		return nil, err
	}

	digestsOfAllDeps := []string{digestsForAllFiles.String()}

	finalDigest, err := NewDigestForContent(strings.NewReader(strings.Join(digestsOfAllDeps, "\n:")))
	if err != nil {
		return nil, err
	}
	return finalDigest, nil
}
