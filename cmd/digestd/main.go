package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	digest "example.com/commerce-order-digest"
)

type service struct {
	infrai *digest.InfraiClient
	store  *digest.UpdateStore
}

func main() {
	client, err := digest.NewInfraiClientFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	publicURL := strings.TrimRight(os.Getenv("PUBLIC_BASE_URL"), "/")
	if publicURL == "" {
		log.Fatal("PUBLIC_BASE_URL is required")
	}

	if len(os.Args) > 1 && os.Args[1] == "schedule" {
		job, err := client.CreateDigestCron(context.Background(), "0 9 * * *", publicURL+"/digest", "daily-commerce-order-digest")
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("scheduled job:", job.JobID)
		return
	}

	svc := &service{infrai: client, store: &digest.UpdateStore{}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /orders", svc.recordOrder)
	mux.HandleFunc("POST /digest", svc.sendDigest)
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	log.Printf("digest service listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func (s *service) recordOrder(w http.ResponseWriter, r *http.Request) {
	var update digest.OrderUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "invalid order update", http.StatusBadRequest)
		return
	}
	if update.OrderID == "" || update.CustomerEmail == "" || update.TotalCents < 0 {
		http.Error(w, "order_id, customer_email, and a non-negative total_cents are required", http.StatusBadRequest)
		return
	}
	s.store.Add(update)
	w.WriteHeader(http.StatusAccepted)
}

func (s *service) sendDigest(w http.ResponseWriter, r *http.Request) {
	updates := s.store.Snapshot()
	batch := digest.BuildReceiptDigest(updates)
	if len(batch.Messages) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	if err := s.infrai.SendDigestBatch(ctx, batch.Messages, batch.IdempotencyKey); err != nil {
		status := http.StatusBadGateway
		var apiErr *digest.InfraiError
		if errors.As(err, &apiErr) && apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500 {
			status = apiErr.HTTPStatus
		}
		http.Error(w, err.Error(), status)
		return
	}
	s.store.Commit(len(updates))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"recipients": len(batch.Messages)})
}
