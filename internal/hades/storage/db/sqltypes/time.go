// Package sqltypes provides sql.Scanner implementations for SQLite date/time
// columns. modernc.org/sqlite returns DATETIME values as Go strings in SQLite
// format ("2006-01-02 15:04:05"), which database/sql cannot scan directly into
// time.Time (it only tries RFC3339). Use Time and NullTime here instead.
package sqltypes

import (
	"fmt"
	"time"
)

var formats = []string{
	"2006-01-02 15:04:05.999999999-07:00", // modernc.org/sqlite default write format
	"2006-01-02 15:04:05-07:00",
	"2006-01-02 15:04:05",
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05Z07:00",
}

// Time is a sql.Scanner for a non-nullable SQLite DATETIME column.
type Time struct{ V time.Time }

func (t *Time) Scan(v any) error {
	if v == nil {
		return nil
	}
	switch s := v.(type) {
	case string:
		for _, f := range formats {
			if parsed, err := time.Parse(f, s); err == nil {
				t.V = parsed
				return nil
			}
		}
		return fmt.Errorf("sqltypes: cannot parse %q as time", s)
	case time.Time:
		t.V = s
		return nil
	}
	return fmt.Errorf("sqltypes: unsupported type %T for Time", v)
}

// NullTime is a sql.Scanner for a nullable SQLite DATETIME column.
type NullTime struct {
	Time  time.Time
	Valid bool
}

func (t *NullTime) Scan(v any) error {
	if v == nil {
		t.Valid = false
		return nil
	}
	t.Valid = true
	switch s := v.(type) {
	case string:
		for _, f := range formats {
			if parsed, err := time.Parse(f, s); err == nil {
				t.Time = parsed
				return nil
			}
		}
		return fmt.Errorf("sqltypes: cannot parse %q as time", s)
	case time.Time:
		t.Time = s
		return nil
	}
	return fmt.Errorf("sqltypes: unsupported type %T for NullTime", v)
}

// Ptr returns a *time.Time pointer, nil when not Valid.
func (t NullTime) Ptr() *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}
