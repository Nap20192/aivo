package sharedkernel

// AggregateRoot collects domain events raised while mutating an
// aggregate, for a caller (app layer) to publish after the transaction
// commits. Embed it in aggregate root entities.
type AggregateRoot struct {
	events []DomainEvent
}

// Raise records e for later publication.
func (a *AggregateRoot) Raise(e DomainEvent) { a.events = append(a.events, e) }

// Events returns the recorded events and clears the buffer.
func (a *AggregateRoot) Events() []DomainEvent {
	out := a.events
	a.events = nil
	return out
}
