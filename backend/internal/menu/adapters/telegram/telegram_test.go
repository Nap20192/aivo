package telegram

import (
	"testing"

	"aivo/internal/domain/menu"

	"uuid"
)

func TestRenderOrder(t *testing.T) {
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	order := domain.Order{
		ID: id,
		Lines: []domain.OrderLine{
			{
				Name: "Burger",
				Qty:  2,
				ChosenOptions: []domain.OrderLineOption{
					{Label: "Large", PriceDeltaCents: 200},
					{Label: "Extra cheese", PriceDeltaCents: 100},
				},
			},
			{
				Name: "Fries",
				Qty:  1,
			},
		},
		Comment: "No onions please",
	}

	got := renderOrder("5", order)
	want := "Table 5 — new order (#11111111-1111-1111-1111-111111111111)\n" +
		"2× Burger (Large, Extra cheese)\n" +
		"1× Fries\n" +
		"Note: No onions please"

	if got != want {
		t.Errorf("renderOrder() =\n%q\nwant\n%q", got, want)
	}
}

func TestRenderOrder_NoCommentNoOptions(t *testing.T) {
	order := domain.Order{
		ID: uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		Lines: []domain.OrderLine{
			{Name: "Soda", Qty: 3},
		},
	}

	got := renderOrder("7", order)
	want := "Table 7 — new order (#22222222-2222-2222-2222-222222222222)\n3× Soda"

	if got != want {
		t.Errorf("renderOrder() =\n%q\nwant\n%q", got, want)
	}
}

func TestRenderServiceRequest(t *testing.T) {
	tests := []struct {
		kind domain.ServiceRequestKind
		want string
	}{
		{domain.CallWaiter, "Table 12 — call waiter"},
		{domain.RequestBill, "Table 12 — request bill"},
	}

	for _, tt := range tests {
		got := renderServiceRequest("12", tt.kind)
		if got != tt.want {
			t.Errorf("renderServiceRequest(%q) = %q, want %q", tt.kind, got, tt.want)
		}
	}
}
