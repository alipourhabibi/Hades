package shake256

import (
	"bytes"
	"fmt"
	"slices"
	"strings"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
)

// DigestFiles computes a single SHAKE-256 digest over a set of files.
// Files are sorted by path before hashing to ensure deterministic output
// regardless of input order.
func DigestFiles(datas []*registryv1.File) (*digest, error) {
	digests := ""

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

	digestOfDigests, err := NewDigestForContent(strings.NewReader(digests))
	if err != nil {
		// Returned, not panicked. This is the only panic on a request path in
		// the server: DigestFiles is called by the upload handler, so a failure
		// here took down the connection serving the push instead of answering
		// it, and the three other NewDigestForContent calls in this same
		// function already return their error.
		return nil, fmt.Errorf("shake256: digest of digests: %w", err)
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
