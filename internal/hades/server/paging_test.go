package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPage_ClampsPageSize(t *testing.T) {
	cases := []struct {
		name      string
		pageSize  int32
		wantLimit int
	}{
		{"zero uses the default", 0, DefaultPageSize},
		{"negative uses the default", -10, DefaultPageSize},
		{"in range is honoured", 25, 25},
		{"at the cap", MaxPageSize, MaxPageSize},
		// Without a ceiling this turns one request into a full table scan.
		{"over the cap is clamped", 1_000_000, MaxPageSize},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			limit, _ := Page(tc.pageSize, "")
			assert.Equal(t, tc.wantLimit, limit)
		})
	}
}

func TestPage_RejectsBadTokens(t *testing.T) {
	cases := []struct {
		name       string
		token      string
		wantOffset int
	}{
		{"empty", "", 0},
		{"valid", "150", 150},
		// strconv.Atoi parses these cleanly, so without an explicit guard they
		// reach the database as a negative OFFSET and error out.
		{"negative", "-1", 0},
		{"large negative", "-999999", 0},
		{"not a number", "abc", 0},
		{"float", "1.5", 0},
		{"injection attempt", "0; DROP TABLE users", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, offset := Page(0, tc.token)
			assert.Equal(t, tc.wantOffset, offset)
			assert.GreaterOrEqual(t, offset, 0, "offset must never be negative")
		})
	}
}

func TestNextPageToken(t *testing.T) {
	// A full page means there may be more rows.
	assert.Equal(t, "100", NextPageToken(50, 50, 50))
	// A short page means the table is exhausted.
	assert.Equal(t, "", NextPageToken(20, 50, 50))
	assert.Equal(t, "", NextPageToken(0, 50, 0))
	assert.Equal(t, "50", NextPageToken(50, 50, 0))
}
