package control

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	frinbox "github.com/SemperSupra/folio-relay/internal/inbox"
	frstate "github.com/SemperSupra/folio-relay/internal/state"
)

const testToken = "0123456789abcdef0123456789abcdef"

func TestIngestInboxArtifactSurvivesRestart(t *testing.T) {
	root := t.TempDir()
	journal := filepath.Join(root, "state", "journal.frj")
	store := filepath.Join(root, "artifacts")
	body := []byte("%PDF-1.4\nfixture\n")
	digest := sha256.Sum256(body)
	hexDigest := hex.EncodeToString(digest[:])
	artifactPath := filepath.Join(store, "sha256", hexDigest[:2], hexDigest)
	if err := os.MkdirAll(filepath.Dir(artifactPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(artifactPath, body, 0o600); err != nil {
		t.Fatal(err)
	}

	state, err := frstate.OpenDurableEngine(journal)
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(state, store, testToken)
	if err != nil {
		t.Fatal(err)
	}
	h := server.Handler()

	payload := map[string]any{
		"aggregate_id":         "ingress/pappl/job-7",
		"semantic_fingerprint": "sha256:semantic",
		"artifact_sha256":      hexDigest,
		"artifact_bytes":       len(body),
		"media_type":           "application/pdf",
		"substrate":            "pappl",
		"substrate_job_id":     "7",
		"copies":               1,
	}
	encoded, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ingest", bytes.NewReader(encoded))
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("Idempotency-Key", "ingress/pappl/job-7")
	resp := httptest.NewRecorder()
	h.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("ingest failed: %d %s", resp.Code, resp.Body.String())
	}
	if err := state.Close(); err != nil {
		t.Fatal(err)
	}

	state, err = frstate.OpenDurableEngine(journal)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	server, err = New(state, store, testToken)
	if err != nil {
		t.Fatal(err)
	}
	h = server.Handler()

	jobsReq := httptest.NewRequest(http.MethodGet, "/api/v1/jobs", nil)
	jobsReq.Header.Set("Authorization", "Bearer "+testToken)
	jobsResp := httptest.NewRecorder()
	h.ServeHTTP(jobsResp, jobsReq)
	if jobsResp.Code != http.StatusOK {
		t.Fatalf("jobs failed: %d %s", jobsResp.Code, jobsResp.Body.String())
	}
	var page struct {
		Items []frinbox.Record `json:"items"`
	}
	if err := json.Unmarshal(jobsResp.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].AggregateID != "ingress/pappl/job-7" {
		t.Fatalf("unexpected inbox: %+v", page.Items)
	}

	artifactReq := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+page.Items[0].JobID+"/artifact", nil)
	artifactReq.Header.Set("Authorization", "Bearer "+testToken)
	artifactResp := httptest.NewRecorder()
	h.ServeHTTP(artifactResp, artifactReq)
	if artifactResp.Code != http.StatusOK {
		t.Fatalf("artifact failed: %d %s", artifactResp.Code, artifactResp.Body.String())
	}
	got, _ := io.ReadAll(artifactResp.Body)
	if !bytes.Equal(got, body) {
		t.Fatalf("artifact changed: %q", got)
	}
}

func TestManagementEndpointsRequireBearerToken(t *testing.T) {
	state, err := frstate.OpenDurableEngine(filepath.Join(t.TempDir(), "journal.frj"))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	server, err := New(state, t.TempDir(), testToken)
	if err != nil {
		t.Fatal(err)
	}
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/v1/jobs", nil))
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.Code)
	}
}

func TestIngestRequiresStagedArtifact(t *testing.T) {
	state, err := frstate.OpenDurableEngine(filepath.Join(t.TempDir(), "journal.frj"))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	server, err := New(state, t.TempDir(), testToken)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"aggregate_id":"ingress/cups/1","semantic_fingerprint":"sha256:x","artifact_sha256":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","artifact_bytes":1,"media_type":"application/pdf","substrate":"cups","substrate_job_id":"1","copies":1}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ingest", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("Idempotency-Key", "ingress/cups/1")
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", resp.Code, resp.Body.String())
	}
}
