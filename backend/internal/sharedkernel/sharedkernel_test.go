package sharedkernel

import (
	"testing"
	"time"
)

type testEvent struct {
	EventBase
}

func (testEvent) Name() string { return "TestHappened" }

func TestIDRoundTrip(t *testing.T) {
	id := NewID()
	parsed, err := ParseID(id.String())
	if err != nil || parsed != id {
		t.Fatalf("ParseID(%s) = %v, %v", id, parsed, err)
	}
}

func TestAggregateRootEvents(t *testing.T) {
	var a AggregateRoot
	a.Raise(testEvent{EventBase{At: time.Now()}})
	a.Raise(testEvent{EventBase{At: time.Now()}})
	if got := a.Events(); len(got) != 2 || got[0].Name() != "TestHappened" {
		t.Fatalf("Events() = %v", got)
	}
	if got := a.Events(); len(got) != 0 {
		t.Fatalf("Events() after drain = %v, want empty", got)
	}
}
