package store

import (
	"errors"

	"github.com/jackc/pgx/v5"
)

// ErrNotFound is returned by single-row lookups that find nothing.
var ErrNotFound = errors.New("store: not found")

func isNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}
