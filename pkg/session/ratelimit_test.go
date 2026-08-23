package session

import (
	"testing"

	"aivo/internal/domain/menu"

	"github.com/google/uuid"
)

func TestAllowOrder_DebouncesWithin30s(t *testing.T) {
	sess := "sess-" + uuid.NewString()

	if !AllowOrder(sess) {
		t.Fatal("first order should be allowed")
	}
	if AllowOrder(sess) {
		t.Fatal("second order within 30s should be blocked")
	}
}

func TestAllowServiceRequest_DedupesUntilAcknowledged(t *testing.T) {
	table := uuid.New()

	if !AllowServiceRequest(table, domain.CallWaiter) {
		t.Fatal("first service request should be allowed")
	}
	if AllowServiceRequest(table, domain.CallWaiter) {
		t.Fatal("second open request of the same kind should be blocked")
	}
	// A different kind on the same table is independent.
	if !AllowServiceRequest(table, domain.RequestBill) {
		t.Fatal("different kind should not be blocked by the other kind's open request")
	}

	MarkAcknowledged(table, domain.CallWaiter)
	if !AllowServiceRequest(table, domain.CallWaiter) {
		t.Fatal("request should be allowed again after acknowledgement")
	}
}

func TestAllowIP_BlocksAfter20PerMinute(t *testing.T) {
	ip := "203.0.113." + uuid.NewString()[:2]

	for i := 1; i <= ipLimit; i++ {
		if !AllowIP(ip) {
			t.Fatalf("call %d should be allowed (limit is %d)", i, ipLimit)
		}
	}
	if AllowIP(ip) {
		t.Fatal("21st call within the window should be blocked")
	}
}
