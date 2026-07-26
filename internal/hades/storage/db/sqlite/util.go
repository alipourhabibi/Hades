package sqlite

import "github.com/google/uuid"

// sqliteUUID normalises a uuid.UUID to the no-hyphen hex format stored in SQLite.
// The schema DEFAULT (lower(hex(randomblob(16)))) produces 32 hex chars without
// hyphens, while uuid.UUID.String() returns the hyphenated form. This helper
// strips hyphens so WHERE id = ? parameters match the stored value.
func sqliteUUID(id uuid.UUID) string {
	s := id.String()
	out := make([]byte, 0, 32)
	for i := range len(s) {
		if s[i] != '-' {
			out = append(out, s[i])
		}
	}
	return string(out)
}
