package types

import (
	"errors"
	"strings"

	"github.com/google/uuid"
)

// NewID creates a new UUIDv4.
func NewID() ID { return uuid.New() }

// ParseID parses a UUID string and enforces lowercase canonical form.
func ParseID(s string) (ID, error) {
	s = strings.TrimSpace(s)
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, errors.New("invalid UUID")
	}
	return id, nil
}

// IsZeroID reports whether the ID is the zero UUID.
func IsZeroID(id ID) bool { return id == uuid.Nil }
