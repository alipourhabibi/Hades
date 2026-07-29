package sqlutil

import "github.com/google/uuid"

// UUID returns the UUID as a 32-char lowercase hex string without hyphens.
// Use this when passing a uuid.UUID to SQLite WHERE clauses because SQLite
// stores IDs as unhyphenated hex while uuid.UUID.String() returns hyphens.
func UUID(id uuid.UUID) string {
	s := id.String()
	out := make([]byte, 0, 32)
	for i := 0; i < len(s); i++ {
		if s[i] != '-' {
			out = append(out, s[i])
		}
	}
	return string(out)
}
