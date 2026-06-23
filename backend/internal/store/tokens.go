package store

import "errors"

// ErrNotFound is returned by getters when no matching row exists.
var ErrNotFound = errors.New("store: not found")
