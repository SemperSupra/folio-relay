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
	frprinter "github.com/SemperSupra/folio-relay/internal/printer"
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
	server, err := New(state, store, testToken, testPrinter(t), Profiles{WindowsIPP: true})
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
	server, err = New(state, store, testToken, testPrinter(t), Profiles{WindowsIPP: true})
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
	server, err := New(state, t.TempDir(), testToken, testPrinter(t), Profiles{WindowsIPP: true})
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
	server, err := New(state, t.TempDir(), testToken, testPrinter(t), Profiles{WindowsIPP: true})
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

func TestCapabilityDocumentUsesCanonicalPrinterIdentity(t *testing.T) {
	state, err := frstate.OpenDurableEngine(filepath.Join(t.TempDir(), "journal.frj"))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	identity := testPrinter(t)
	server, err := New(state, t.TempDir(), testToken, identity, Profiles{WindowsIPP: true, AirPrint: false})
	if err != nil {
		t.Fatal(err)
	}
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/.well-known/foliorelay", nil))
	if resp.Code != http.StatusOK {
		t.Fatalf("capability document failed: %d %s", resp.Code, resp.Body.String())
	}
	var doc struct {
		Product  string             `json:"product"`
		Printer  frprinter.Identity `json:"printer"`
		Profiles Profiles           `json:"profiles"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Product != "FolioRelay" || doc.Printer != identity || !doc.Profiles.WindowsIPP || doc.Profiles.AirPrint {
		t.Fatalf("unexpected capability document: %+v", doc)
	}
}

func testPrinter(t *testing.T) frprinter.Identity {
	t.Helper()
	identity, err := frprinter.New("FolioRelay", "Lab", "ipp://foliorelay.local:8634/printers/FolioRelay")
	if err != nil {
		t.Fatal(err)
	}
	return identity
}

func TestOpenAPIAndSelfTestUseSharedProductSurface(t *testing.T) {
	state, err := frstate.OpenDurableEngine(filepath.Join(t.TempDir(), "journal.frj"))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	server, err := New(state, t.TempDir(), testToken, testPrinter(t), Profiles{WindowsIPP: true})
	if err != nil {
		t.Fatal(err)
	}

	openapi := httptest.NewRecorder()
	server.Handler().ServeHTTP(openapi, httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil))
	if openapi.Code != http.StatusOK || !bytes.Contains(openapi.Body.Bytes(), []byte("openapi: 3.1.0")) {
		t.Fatalf("OpenAPI surface unavailable: %d %q", openapi.Code, openapi.Body.String())
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/self-test", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("self-test failed: %d %s", resp.Code, resp.Body.String())
	}
	var receipt struct {
		Status string `json:"status"`
		Checks []struct {
			Code string `json:"code"`
			Status string `json:"status"`
		} `json:"checks"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.Status != "warn" {
		t.Fatalf("expected pre-AirPrint warning, got %+v", receipt)
	}
	seenIdentity := false
	seenAirPrint := false
	for _, check := range receipt.Checks {
		if check.Code == "printer.identity" && check.Status == "pass" {
			seenIdentity = true
		}
		if check.Code == "profiles.airprint" && check.Status == "warn" {
			seenAirPrint = true
		}
	}
	if !seenIdentity || !seenAirPrint {
		t.Fatalf("missing structured self-test evidence: %+v", receipt)
	}
}

func TestWebSessionAuthenticatesSameAPIWithoutExposingBearerToken(t *testing.T) {
	state, err := frstate.OpenDurableEngine(filepath.Join(t.TempDir(), "journal.frj"))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	server, err := New(state, t.TempDir(), testToken, testPrinter(t), Profiles{WindowsIPP: true})
	if err != nil {
		t.Fatal(err)
	}
	h := server.Handler()

	login := httptest.NewRequest(http.MethodPost, "/auth/session",
		bytes.NewBufferString(`{"token":"`+testToken+`"}`))
	login.Header.Set("Content-Type", "application/json")
	loginResp := httptest.NewRecorder()
	h.ServeHTTP(loginResp, login)
	if loginResp.Code != http.StatusNoContent {
		t.Fatalf("login failed: %d %s", loginResp.Code, loginResp.Body.String())
	}
	cookies := loginResp.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != sessionCookieName || !cookies[0].HttpOnly {
		t.Fatalf("unexpected session cookie: %+v", cookies)
	}
	if cookies[0].Value == testToken {
		t.Fatal("session cookie exposed management bearer token")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs", nil)
	req.AddCookie(cookies[0])
	resp := httptest.NewRecorder()
	h.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("session did not authenticate API: %d %s", resp.Code, resp.Body.String())
	}
}

func TestWebSessionStatusIsPublicAndReflectsBrowserSession(t *testing.T) {
	state, err := frstate.OpenDurableEngine(filepath.Join(t.TempDir(), "journal.frj"))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	server, err := New(state, t.TempDir(), testToken, testPrinter(t), Profiles{WindowsIPP: true})
	if err != nil {
		t.Fatal(err)
	}
	h := server.Handler()

	unauth := httptest.NewRecorder()
	h.ServeHTTP(unauth, httptest.NewRequest(http.MethodGet, "/auth/session", nil))
	if unauth.Code != http.StatusOK {
		t.Fatalf("session status failed: %d %s", unauth.Code, unauth.Body.String())
	}
	var unauthStatus struct {
		Authenticated bool `json:"authenticated"`
	}
	if err := json.Unmarshal(unauth.Body.Bytes(), &unauthStatus); err != nil {
		t.Fatal(err)
	}
	if unauthStatus.Authenticated {
		t.Fatal("unauthenticated browser reported authenticated")
	}
	if got := unauth.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("session status must be no-store, got %q", got)
	}

	login := httptest.NewRequest(http.MethodPost, "/auth/session",
		bytes.NewBufferString(`{"token":"`+testToken+`"}`))
	login.Header.Set("Content-Type", "application/json")
	loginResp := httptest.NewRecorder()
	h.ServeHTTP(loginResp, login)
	if loginResp.Code != http.StatusNoContent {
		t.Fatalf("login failed: %d %s", loginResp.Code, loginResp.Body.String())
	}
	cookies := loginResp.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected one session cookie, got %+v", cookies)
	}

	authReq := httptest.NewRequest(http.MethodGet, "/auth/session", nil)
	authReq.AddCookie(cookies[0])
	auth := httptest.NewRecorder()
	h.ServeHTTP(auth, authReq)
	if auth.Code != http.StatusOK {
		t.Fatalf("authenticated session status failed: %d %s", auth.Code, auth.Body.String())
	}
	var authStatus struct {
		Authenticated bool `json:"authenticated"`
	}
	if err := json.Unmarshal(auth.Body.Bytes(), &authStatus); err != nil {
		t.Fatal(err)
	}
	if !authStatus.Authenticated {
		t.Fatal("valid browser session was not recognized")
	}
}

func TestWebSessionRejectsWrongCredential(t *testing.T) {
	state, err := frstate.OpenDurableEngine(filepath.Join(t.TempDir(), "journal.frj"))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	server, err := New(state, t.TempDir(), testToken, testPrinter(t), Profiles{WindowsIPP: true})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/auth/session",
		bytes.NewBufferString(`{"token":"not-the-management-token"}`))
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.Code)
	}
}

func TestWebPortalIsPublicButContainsNoCredential(t *testing.T) {
	state, err := frstate.OpenDurableEngine(filepath.Join(t.TempDir(), "journal.frj"))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	server, err := New(state, t.TempDir(), testToken, testPrinter(t), Profiles{WindowsIPP: true})
	if err != nil {
		t.Fatal(err)
	}
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/", nil))
	if resp.Code != http.StatusOK || !bytes.Contains(resp.Body.Bytes(), []byte("FolioRelay")) {
		t.Fatalf("web portal unavailable: %d %s", resp.Code, resp.Body.String())
	}
	if bytes.Contains(resp.Body.Bytes(), []byte(testToken)) {
		t.Fatal("web portal leaked management credential")
	}
	if got := resp.Header().Get("Content-Security-Policy"); got == "" {
		t.Fatal("web portal missing content security policy")
	}
}

func TestWebFaviconProbeIsNoiseFree(t *testing.T) {
	state, err := frstate.OpenDurableEngine(filepath.Join(t.TempDir(), "journal.frj"))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	server, err := New(state, t.TempDir(), testToken, testPrinter(t), Profiles{WindowsIPP: true})
	if err != nil {
		t.Fatal(err)
	}
	resp := httptest.NewRecorder()
	server.Handler().ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/favicon.ico", nil))
	if resp.Code != http.StatusNoContent {
		t.Fatalf("favicon probe should be noise-free: %d %s", resp.Code, resp.Body.String())
	}
	if got := resp.Header().Get("Content-Security-Policy"); got == "" {
		t.Fatal("favicon response missing web security headers")
	}
}
