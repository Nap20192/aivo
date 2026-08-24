// Package sharedkernel holds the DDD building blocks shared by every
// bounded context (internal/domain/*): identity, base entity, aggregate
// root, and domain events. Modeled after go-coffeeshop's shared_kernel.
// It must not import any context package — dependencies point inward.
package sharedkernel

import (
	"database/sql"
	"time"
	"uuid"
)

// ID is the identity type for all entities. An alias (not a defined
// type) so existing uuid.UUID code interoperates without conversions.
type ID = uuid.UUID

// NewID returns a fresh random ID.
func NewID() ID { return uuid.New() }

// ParseID parses an ID from its string form.
func ParseID(s string) (ID, error) { return uuid.Parse(s) }

// NullID is a nullable ID for scanning NULLable uuid columns (sqlc uses
// it for nullable uuid overrides).
type NullID = sql.Null[ID]

// Entity is the base for domain entities that carry identity and a
// creation timestamp. Embed it in new entities; existing structs with
// their own ID/CreatedAt fields are equivalent and need not migrate.
type Entity struct {
	ID        ID
	CreatedAt time.Time
}
