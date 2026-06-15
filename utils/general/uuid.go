// Package general provides miscellaneous utility functions.
package general

import "github.com/google/uuid"

// ToDashless returns the UUID as a 32-character hex string without dashes.
func ToDashless(id uuid.UUID) string {
	s := id.String()
	return s[0:8] + s[9:13] + s[14:18] + s[19:23] + s[24:]
}
