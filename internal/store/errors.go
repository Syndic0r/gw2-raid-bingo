package store

import (
	"errors"

	sqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// isUniqueViolation reports whether err is a SQLite UNIQUE constraint failure,
// so callers can translate a race or duplicate into a friendly message.
func isUniqueViolation(err error) bool {
	var se *sqlite.Error
	if errors.As(err, &se) {
		code := se.Code()
		return code == sqlite3.SQLITE_CONSTRAINT_UNIQUE ||
			code == sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY
	}
	return false
}
