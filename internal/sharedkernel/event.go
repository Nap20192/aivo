package sharedkernel

import "time"

// DomainEvent is something that happened in a bounded context that other
// parts of the system may react to. Name is the stable event identity
// (e.g. "OrderPlaced"); OccurredAt is when it happened.
type DomainEvent interface {
	Name() string
	OccurredAt() time.Time
}

// EventBase implements the OccurredAt half of DomainEvent; concrete
// events embed it and add Name plus their payload fields.
type EventBase struct {
	At time.Time
}

// OccurredAt implements DomainEvent.
func (e EventBase) OccurredAt() time.Time { return e.At }
