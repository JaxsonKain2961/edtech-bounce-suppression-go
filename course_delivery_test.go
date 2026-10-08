package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHardBounceSuppressesLearnerAndChangesEducatorReport(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodPost || r.URL.Path != "/v1/email/suppression/add" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true,"data":{}}`))
	}))
	defer server.Close()

	now := time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC)
	client := &InfraiClient{key: "test-key", baseURL: server.URL, httpClient: server.Client()}
	service := NewDeliveryService(client)
	service.deliveries["msg-42"] = &LearnerDelivery{Course: "Algebra I", Learner: "Mina", Email: "mina@example.edu", Deadline: now.Add(48 * time.Hour), MessageID: "msg-42"}

	delivery, err := service.RecordBounce(context.Background(), BounceNotice{MessageID: "msg-42", Kind: "hard"})
	if err != nil {
		t.Fatal(err)
	}
	if !delivery.Suppressed || requests != 1 {
		t.Fatalf("suppressed=%v requests=%d", delivery.Suppressed, requests)
	}
	report := service.Report("Algebra I", now)
	if report.SuppressedLearners != 1 || report.DueSoon != 0 {
		t.Fatalf("unexpected report: %+v", report)
	}
}
