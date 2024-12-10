package shake256

import (
	"encoding/hex"
	"fmt"
	"io"
	"slices"

	"golang.org/x/crypto/sha3"
)

const shake256Length = 64

type digest struct {
	value       []byte
	stringValue string
}

func (d *digest) String() string {
	return d.stringValue
}

// NewDigest returns a new Digest for the content read from the Reader.
func NewDigestForContent(reader io.Reader) (*digest, error) {
	shakeHash := sha3.NewShake256()
	shakeHash.Reset()
	if _, err := io.Copy(shakeHash, reader); err != nil {
		return nil, err
	}
	value := make([]byte, shake256Length)
	if _, err := shakeHash.Read(value); err != nil {
		// sha3.ShakeHash never errors or short reads. Something horribly wrong
		// happened if your computer ended up here.
		return nil, err
	}
	return newDigest(value)
}

func newDigest(value []byte) (*digest, error) {
	if len(value) != shake256Length {
		return nil, fmt.Errorf("invalid shake256 digest value: expected %d bytes, got %d", shake256Length, len(value))
	}
	return &digest{
		value:       value,
		stringValue: "shake256" + ":" + hex.EncodeToString(value),
	}, nil
}

func (d *digest) Value() []byte {
	return slices.Clone(d.value)
}
