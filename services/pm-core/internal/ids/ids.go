// Package ids generates pro_manage identifiers: UUIDv7 primary keys (time-ordered,
// FSD §03 "Identifiers and versioning").
package ids

import "github.com/google/uuid"

// New returns a UUIDv7. It panics only if the system clock or entropy source is
// unusable, which is not a condition the application can continue under.
func New() uuid.UUID {
	id, err := uuid.NewV7()
	if err != nil {
		panic("ids: uuidv7: " + err.Error())
	}
	return id
}

// Parse accepts the canonical 36-character form.
func Parse(s string) (uuid.UUID, error) { return uuid.Parse(s) }
