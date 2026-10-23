package digest

import (
	"strings"
	"testing"
)

func TestBuildReceiptDigest(t *testing.T) {
	tests := []struct {
		name       string
		updates    []OrderUpdate
		wantEmails int
		wantParts  []string
	}{
		{
			name: "groups the order lifecycle for each subscriber",
			updates: []OrderUpdate{
				{OrderID: "ord-42", CustomerEmail: "buyer@example.com", Subscriber: true, Stage: StageReceipt, TotalCents: 2599},
				{OrderID: "ord-42", CustomerEmail: "buyer@example.com", Subscriber: true, Stage: StageCheckout, TotalCents: 2599},
				{OrderID: "ord-77", CustomerEmail: "other@example.com", Subscriber: true, Stage: StageFulfilled, TotalCents: 4800},
			},
			wantEmails: 2,
			wantParts:  []string{"ord-42: checkout, $25.99", "ord-42: receipt, $25.99"},
		},
		{
			name: "excludes customers who did not subscribe",
			updates: []OrderUpdate{
				{OrderID: "ord-88", CustomerEmail: "quiet@example.com", Subscriber: false, Stage: StageFulfilled, TotalCents: 1200},
			},
			wantEmails: 0,
		},
		{
			name: "excludes unrelated order states",
			updates: []OrderUpdate{
				{OrderID: "ord-99", CustomerEmail: "buyer@example.com", Subscriber: true, Stage: "inventory_check", TotalCents: 3000},
			},
			wantEmails: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildReceiptDigest(tt.updates)
			if len(got.Messages) != tt.wantEmails {
				t.Fatalf("got %d emails, want %d", len(got.Messages), tt.wantEmails)
			}
			for _, part := range tt.wantParts {
				if !strings.Contains(got.Messages[0].HTML, part) {
					t.Errorf("digest HTML does not contain %q", part)
				}
			}
			if got.IdempotencyKey == "" {
				t.Fatal("digest key is empty")
			}
		})
	}
}
