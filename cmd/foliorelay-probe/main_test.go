package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestReadinessContractIsNoContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := &http.Client{Timeout: time.Second}
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("unexpected readiness status %d", resp.StatusCode)
	}
}
