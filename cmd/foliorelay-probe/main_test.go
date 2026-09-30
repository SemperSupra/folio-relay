package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCheckAcceptsNoContentReadiness(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	if err := check(&http.Client{Timeout: time.Second}, server.URL); err != nil {
		t.Fatal(err)
	}
}

func TestCheckRejectsNonReadyStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	err := check(&http.Client{Timeout: time.Second}, server.URL)
	if err == nil || !strings.Contains(err.Error(), "503") {
		t.Fatalf("expected HTTP 503 failure, got %v", err)
	}
}
