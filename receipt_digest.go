package digest

import (
	"crypto/sha256"
	"fmt"
	"html"
	"sort"
	"strings"
	"sync"
)

type OrderStage string

const (
	StageCheckout  OrderStage = "checkout"
	StageFulfilled OrderStage = "fulfilled"
	StageReceipt   OrderStage = "receipt"
)

type OrderUpdate struct {
	OrderID       string     `json:"order_id"`
	CustomerEmail string     `json:"customer_email"`
	Subscriber    bool       `json:"subscriber"`
	Stage         OrderStage `json:"stage"`
	TotalCents    int        `json:"total_cents"`
}

type Digest struct {
	Messages       []EmailMessage
	IdempotencyKey string
}

func BuildReceiptDigest(updates []OrderUpdate) Digest {
	grouped := make(map[string][]OrderUpdate)
	for _, update := range updates {
		if !update.Subscriber || update.CustomerEmail == "" || update.OrderID == "" {
			continue
		}
		switch update.Stage {
		case StageCheckout, StageFulfilled, StageReceipt:
			grouped[update.CustomerEmail] = append(grouped[update.CustomerEmail], update)
		}
	}

	recipients := make([]string, 0, len(grouped))
	for recipient := range grouped {
		recipients = append(recipients, recipient)
	}
	sort.Strings(recipients)

	messages := make([]EmailMessage, 0, len(recipients))
	fingerprint := sha256.New()
	for _, recipient := range recipients {
		rows := grouped[recipient]
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].OrderID == rows[j].OrderID {
				return rows[i].Stage < rows[j].Stage
			}
			return rows[i].OrderID < rows[j].OrderID
		})
		var list strings.Builder
		list.WriteString("<h1>Your order digest</h1><ul>")
		for _, row := range rows {
			fmt.Fprintf(&list, "<li>%s: %s, $%.2f</li>", html.EscapeString(row.OrderID), html.EscapeString(string(row.Stage)), float64(row.TotalCents)/100)
			fmt.Fprintf(fingerprint, "%s\x00%s\x00%s\x00%d\n", recipient, row.OrderID, row.Stage, row.TotalCents)
		}
		list.WriteString("</ul>")
		messages = append(messages, EmailMessage{To: recipient, Subject: "Checkout, fulfillment, and receipt updates", HTML: list.String()})
	}

	return Digest{Messages: messages, IdempotencyKey: fmt.Sprintf("order-digest-%x", fingerprint.Sum(nil))}
}

type UpdateStore struct {
	mu      sync.Mutex
	pending []OrderUpdate
}

func (s *UpdateStore) Add(update OrderUpdate) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending = append(s.pending, update)
}

func (s *UpdateStore) Snapshot() []OrderUpdate {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]OrderUpdate(nil), s.pending...)
}

func (s *UpdateStore) Commit(count int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if count > len(s.pending) {
		count = len(s.pending)
	}
	s.pending = append([]OrderUpdate(nil), s.pending[count:]...)
}
