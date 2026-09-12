// Package sqlutil holds the small conversions the SQLite backend needs.
package sqlutil

import (
	"strings"

	"github.com/google/uuid"
)

// UUID returns the UUID as a 32-char lowercase hex string without hyphens.
//
// SQLite stores ids in that form (the column defaults are
// lower(hex(randomblob(16)))) while uuid.UUID.String() returns the hyphenated
// form, so every id bound into a SQLite query goes through here. Normalising
// the parameter in Go rather than the column in SQL matters: wrapping an
// indexed primary key in a REPLACE call makes the index unusable and turns
// the lookup into a full table scan.
func UUID(id uuid.UUID) string {
	return ID(id.String())
}

// ID normalises an identifier that arrives as a string, which is the shape the
// protobuf API uses. It accepts either spelling and returns the dashless one.
//
// Only a well-formed 36-character canonical UUID is rewritten. Stripping every
// hyphen from every string would corrupt any identifier that is not a UUID,
// which is a wrong answer rather than an unmatched one, and this function is
// called on the way into a WHERE clause where a wrong answer is silent.
func ID(id string) string {
	if len(id) != 36 || id[8] != '-' || id[13] != '-' || id[18] != '-' || id[23] != '-' {
		return id
	}
	stripped := strings.ReplaceAll(id, "-", "")
	if len(stripped) != 32 {
		return id
	}
	for i := 0; i < 32; i++ {
		c := stripped[i]
		if !isHexDigit(c) {
			return id
		}
	}
	return stripped
}

// Canonical returns an identifier in the hyphenated 8-4-4-4-12 form.
//
// SQLite stores ids without hyphens, but the API must not expose that: a
// client comparing an id it received from one call against one it received
// from another would otherwise see two spellings of the same value depending
// on which code path answered, and the same call would return different text
// on PostgreSQL. Storage layout is an implementation detail; the wire format
// is a contract. Every id scanned out of SQLite goes through here.
//
// Anything that is not exactly 32 hex characters is returned unchanged.
func Canonical(id string) string {
	if len(id) != 32 {
		return id
	}
	for i := 0; i < 32; i++ {
		c := id[i]
		if !isHexDigit(c) {
			return id
		}
	}
	var b strings.Builder
	b.Grow(36)
	b.WriteString(id[0:8])
	b.WriteByte('-')
	b.WriteString(id[8:12])
	b.WriteByte('-')
	b.WriteString(id[12:16])
	b.WriteByte('-')
	b.WriteString(id[16:20])
	b.WriteByte('-')
	b.WriteString(id[20:32])
	return b.String()
}

// LikePrefix escapes a caller-supplied string for use as the prefix of a LIKE
// pattern, and returns it together with the escape character to declare.
//
// Without this a search term of "%" matches every row: for a commit hash-prefix
// lookup that returns an arbitrary commit to a caller who supplied no real
// prefix, and for a list endpoint it is a full-table wildcard scan. Backslash
// is escaped first, otherwise it would double-escape the escapes added after.
func LikePrefix(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "%", `\%`)
	s = strings.ReplaceAll(s, "_", `\_`)
	return s
}

// isHexDigit reports whether c is a hexadecimal digit in either case.
func isHexDigit(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}
