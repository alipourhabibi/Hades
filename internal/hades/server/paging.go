package server

import "strconv"

const (
	// DefaultPageSize is used when a request asks for no particular size.
	DefaultPageSize = 50
	// MaxPageSize caps a single page. Without a ceiling a caller can request an
	// unbounded result set and turn one request into a full table scan.
	MaxPageSize = 100
)

// Page converts a request's page_size and page_token into a SQL limit and
// offset.
//
// pageSize is clamped to [1, MaxPageSize]; zero or negative means the default.
// pageToken is an opaque offset cursor. A token that is not a non-negative
// integer is treated as the first page rather than being passed through to the
// query: "-1" parses cleanly with strconv.Atoi and would otherwise reach the
// database as a negative OFFSET.
func Page(pageSize int32, pageToken string) (limit, offset int) {
	limit = int(pageSize)
	if limit <= 0 {
		limit = DefaultPageSize
	}
	if limit > MaxPageSize {
		limit = MaxPageSize
	}

	if pageToken != "" {
		if n, err := strconv.Atoi(pageToken); err == nil && n > 0 {
			offset = n
		}
	}
	return limit, offset
}

// NextPageToken returns the cursor for the page after the one just returned,
// or "" when the returned page was the last one.
//
// returned is the number of rows the query produced, before any post-filtering:
// a short page means the underlying table is exhausted.
func NextPageToken(returned, limit, offset int) string {
	if returned < limit {
		return ""
	}
	return strconv.Itoa(offset + limit)
}
