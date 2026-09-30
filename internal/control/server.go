package control

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	frapi "github.com/SemperSupra/folio-relay/api"
	frinbox "github.com/SemperSupra/folio-relay/internal/inbox"
	frprinter "github.com/SemperSupra/folio-relay/internal/printer"
	frsecurity "github.com/SemperSupra/folio-relay/internal/security"
	frstate "github.com/SemperSupra/folio-relay/internal/state"
	frweb "github.com/SemperSupra/folio-relay/internal/webui"
)

const (
	sessionCookieName = "foliorelay_session"
	sessionLifetime   = 12 * time.Hour
)

type Profiles struct {
	WindowsIPP bool `json:"windows_ipp"`
	AirPrint   bool `json:"airprint"`
}

type Server struct {
	state         *frstate.DurableEngine
	artifactStore string
	token         string
	printer       frprinter.Identity
	profiles      Profiles
}

type ingestRequest struct {
	AggregateID         string `json:"aggregate_id"`
	IdempotencyKey      string `json:"idempotency_key,omitempty"`
	SemanticFingerprint string `json:"semantic_fingerprint"`
	ArtifactSHA256      string `json:"artifact_sha256"`
	ArtifactBytes       int64  `json:"artifact_bytes"`
	MediaType           string `json:"media_type"`
	Substrate           string `json:"substrate"`
	SubstrateJobID      string `json:"substrate_job_id"`
	Copies              int64  `json:"copies"`
}

func New(state *frstate.DurableEngine, artifactStore, token string, printer frprinter.Identity, profiles Profiles) (*Server, error) {
	if state == nil {
		return nil, errors.New("durable state is required")
	}
	if artifactStore == "" {
		return nil, errors.New("artifact store is required")
	}
	if len(token) < 16 {
		return nil, errors.New("management token must be at least 16 bytes")
	}
	if err := printer.Validate(); err != nil {
		return nil, fmt.Errorf("invalid printer identity: %w", err)
	}
	if err := os.MkdirAll(artifactStore, 0o700); err != nil {
		return nil, fmt.Errorf("create artifact store: %w", err)
	}
	return &Server{
		state: state, artifactStore: artifactStore, token: token,
		printer: printer, profiles: profiles,
	}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /readyz", s.handleReady)
	mux.HandleFunc("GET /.well-known/foliorelay", s.handleCapabilities)
	mux.HandleFunc("GET /openapi.yaml", s.handleOpenAPI)
	mux.HandleFunc("GET /{$}", s.handleWebIndex)
	mux.HandleFunc("GET /app.js", s.handleWebAppJS)
	mux.HandleFunc("GET /style.css", s.handleWebStyle)
	mux.HandleFunc("POST /auth/session", s.handleCreateSession)
	mux.HandleFunc("POST /auth/logout", s.handleDeleteSession)
	mux.Handle("GET /api/v1/status", s.requireAuth(http.HandlerFunc(s.handleStatus)))
	mux.Handle("GET /api/v1/printer", s.requireAuth(http.HandlerFunc(s.handlePrinter)))
	mux.Handle("POST /api/v1/self-test", s.requireAuth(http.HandlerFunc(s.handleSelfTest)))
	mux.Handle("GET /api/v1/jobs", s.requireAuth(http.HandlerFunc(s.handleJobs)))
	mux.Handle("GET /api/v1/jobs/{job_id}", s.requireAuth(http.HandlerFunc(s.handleJob)))
	mux.Handle("GET /api/v1/jobs/{job_id}/artifact", s.requireAuth(http.HandlerFunc(s.handleArtifact)))
	mux.Handle("POST /api/v1/ingest", s.requireAuth(http.HandlerFunc(s.handleIngest)))
	// Compatibility alias for the existing print adapter while the product API
	// replaces the qualification fixture.
	mux.Handle("POST /v1/ingest", s.requireAuth(http.HandlerFunc(s.handleIngest)))
	return mux
}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.authorized(r) {
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) authorized(r *http.Request) bool {
	const prefix = "Bearer "
	if header := r.Header.Get("Authorization"); strings.HasPrefix(header, prefix) {
		got := strings.TrimPrefix(header, prefix)
		if len(got) == len(s.token) &&
			subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) == 1 {
			return true
		}
	}
	cookie, err := r.Cookie(sessionCookieName)
	return err == nil && s.validSession(cookie.Value, time.Now())
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
	decoder.DisallowUnknownFields()
	var body struct {
		Token string `json:"token"`
	}
	if err := decoder.Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid login request")
		return
	}
	if len(body.Token) != len(s.token) ||
		subtle.ConstantTimeCompare([]byte(body.Token), []byte(s.token)) != 1 {
		writeError(w, http.StatusUnauthorized, "invalid_credential", "invalid management credential")
		return
	}
	expires := time.Now().Add(sessionLifetime)
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: s.sessionValue(expires),
		Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode,
		Secure: r.TLS != nil, Expires: expires, MaxAge: int(sessionLifetime.Seconds()),
	})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: "", Path: "/", HttpOnly: true,
		SameSite: http.SameSiteStrictMode, Secure: r.TLS != nil,
		MaxAge: -1, Expires: time.Unix(1, 0),
	})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) sessionValue(expires time.Time) string {
	payload := strconv.FormatInt(expires.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(s.token))
	_, _ = mac.Write([]byte(payload))
	return payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *Server) validSession(value string, now time.Time) bool {
	payload, signature, ok := strings.Cut(value, ".")
	if !ok || payload == "" || signature == "" {
		return false
	}
	expiresUnix, err := strconv.ParseInt(payload, 10, 64)
	if err != nil {
		return false
	}
	expires := time.Unix(expiresUnix, 0)
	if !expires.After(now) || expires.After(now.Add(sessionLifetime+time.Minute)) {
		return false
	}
	got, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(s.token))
	_, _ = mac.Write([]byte(payload))
	return hmac.Equal(got, mac.Sum(nil))
}

func webSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; connect-src 'self'; img-src 'self'; style-src 'self'; script-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
}

func (s *Server) handleWebIndex(w http.ResponseWriter, _ *http.Request) {
	webSecurityHeaders(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(frweb.Index)
}

func (s *Server) handleWebAppJS(w http.ResponseWriter, _ *http.Request) {
	webSecurityHeaders(w)
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(frweb.AppJS)
}

func (s *Server) handleWebStyle(w http.ResponseWriter, _ *http.Request) {
	webSecurityHeaders(w)
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(frweb.StyleCSS)
}

func (s *Server) handleReady(w http.ResponseWriter, _ *http.Request) {
	if _, err := frinbox.Project(s.state.Records()); err != nil {
		http.Error(w, "durable inbox projection unavailable", http.StatusServiceUnavailable)
		return
	}
	info, err := os.Stat(s.artifactStore)
	if err != nil || !info.IsDir() {
		http.Error(w, "artifact store unavailable", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCapabilities(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"product":     "FolioRelay",
		"api_version": "v1",
		"status":      "ready",
		"printer":     s.printer,
		"profiles":    s.profiles,
		"links": map[string]string{
			"openapi":   "/openapi.yaml",
			"status":    "/api/v1/status",
			"jobs":      "/api/v1/jobs",
			"self_test": "/api/v1/self-test",
		},
	})
}

func (s *Server) handleOpenAPI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(frapi.OpenAPI)
}

type selfTestCheck struct {
	Code        string `json:"code"`
	Status      string `json:"status"`
	Message     string `json:"message"`
	Detail      string `json:"detail,omitempty"`
	Remediation string `json:"remediation,omitempty"`
}

func (s *Server) handleSelfTest(w http.ResponseWriter, _ *http.Request) {
	checks := make([]selfTestCheck, 0, 4)
	overall := "pass"

	if err := s.printer.Validate(); err != nil {
		overall = "fail"
		checks = append(checks, selfTestCheck{
			Code: "printer.identity", Status: "fail",
			Message: "Canonical printer identity is invalid.",
			Detail: err.Error(),
			Remediation: "Repair the durable printer identity before admitting print jobs.",
		})
	} else {
		checks = append(checks, selfTestCheck{
			Code: "printer.identity", Status: "pass",
			Message: "Canonical printer identity is valid.",
		})
	}

	if _, err := frinbox.Project(s.state.Records()); err != nil {
		overall = "fail"
		checks = append(checks, selfTestCheck{
			Code: "state.inbox_projection", Status: "fail",
			Message: "Durable Inbox projection cannot be reconstructed.",
			Detail: err.Error(),
			Remediation: "Preserve the journal and inspect the first invalid durable Inbox record.",
		})
	} else {
		checks = append(checks, selfTestCheck{
			Code: "state.inbox_projection", Status: "pass",
			Message: "Durable Inbox projection is reconstructable.",
		})
	}

	info, err := os.Stat(s.artifactStore)
	if err != nil || !info.IsDir() {
		overall = "fail"
		detail := "artifact store is not a directory"
		if err != nil {
			detail = err.Error()
		}
		checks = append(checks, selfTestCheck{
			Code: "storage.artifact_store", Status: "fail",
			Message: "Artifact store is unavailable.",
			Detail: detail,
			Remediation: "Restore the configured FolioRelay data dataset/mount before accepting jobs.",
		})
	} else {
		checks = append(checks, selfTestCheck{
			Code: "storage.artifact_store", Status: "pass",
			Message: "Artifact store is available.",
		})
	}

	checks = append(checks, selfTestCheck{
		Code: "profiles.windows_ipp", Status: "pass",
		Message: "Windows inbox IPP profile is enabled.",
	})
	if s.profiles.AirPrint {
		checks = append(checks, selfTestCheck{
			Code: "profiles.airprint", Status: "pass",
			Message: "AirPrint profile is enabled by qualified configuration.",
		})
	} else {
		checks = append(checks, selfTestCheck{
			Code: "profiles.airprint", Status: "warn",
			Message: "AirPrint profile is not yet enabled.",
			Remediation: "Enable only after the selected substrate passes the AirPrint MVP profile gate.",
		})
		if overall == "pass" {
			overall = "warn"
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": overall,
		"checks": checks,
	})
}

func (s *Server) handlePrinter(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"identity": s.printer,
		"public_uri": s.printer.URI(),
		"profiles": s.profiles,
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	jobs, err := frinbox.Project(s.state.Records())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "inbox_projection_failed", err.Error())
		return
	}
	var last any
	if len(jobs) > 0 {
		last = jobs[0].AcceptedAt
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":                   "ready",
		"inbox_jobs":               len(jobs),
		"last_successful_print_at": last,
	})
}

func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := frinbox.Project(s.state.Records())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "inbox_projection_failed", err.Error())
		return
	}
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 200 {
			writeError(w, http.StatusBadRequest, "invalid_limit", "limit must be between 1 and 200")
			return
		}
		limit = parsed
	}
	offset, err := decodeCursor(r.URL.Query().Get("cursor"))
	if err != nil || offset > len(jobs) {
		writeError(w, http.StatusBadRequest, "invalid_cursor", "cursor is invalid")
		return
	}
	end := offset + limit
	if end > len(jobs) {
		end = len(jobs)
	}
	var next any
	if end < len(jobs) {
		next = encodeCursor(end)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":       jobs[offset:end],
		"next_cursor": next,
	})
}

func (s *Server) handleJob(w http.ResponseWriter, r *http.Request) {
	record, ok, err := s.findJob(r.PathValue("job_id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "inbox_projection_failed", err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "job_not_found", "job not found")
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (s *Server) handleArtifact(w http.ResponseWriter, r *http.Request) {
	record, ok, err := s.findJob(r.PathValue("job_id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "inbox_projection_failed", err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "job_not_found", "job not found")
		return
	}
	path, err := s.artifactPath(record.ArtifactSHA256)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "artifact_reference_invalid", err.Error())
		return
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusNotFound, "artifact_not_found", "artifact not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "artifact_open_failed", "artifact unavailable")
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		writeError(w, http.StatusInternalServerError, "artifact_invalid", "artifact unavailable")
		return
	}
	w.Header().Set("Content-Type", record.MediaType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, record.JobID))
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, file)
}

func (s *Server) handleIngest(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	var req ingestRequest
	if err := decoder.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid ingest request")
		return
	}
	idempotencyKey := r.Header.Get("Idempotency-Key")
	if idempotencyKey == "" {
		idempotencyKey = req.IdempotencyKey
	}
	if req.IdempotencyKey != "" && r.Header.Get("Idempotency-Key") != "" &&
		req.IdempotencyKey != r.Header.Get("Idempotency-Key") {
		writeError(w, http.StatusBadRequest, "idempotency_key_mismatch", "idempotency keys disagree")
		return
	}
	if req.AggregateID == "" || idempotencyKey == "" || req.SemanticFingerprint == "" ||
		req.MediaType == "" || req.Substrate == "" || req.SubstrateJobID == "" ||
		req.Copies < 1 {
		writeError(w, http.StatusBadRequest, "missing_required_field", "missing or invalid required field")
		return
	}
	if strings.ContainsAny(req.MediaType, "\r\n\x00") {
		writeError(w, http.StatusBadRequest, "invalid_media_type", "invalid media type")
		return
	}
	if err := frsecurity.ValidateAdmission(
		frsecurity.AdmissionRequest{Bytes: req.ArtifactBytes, Copies: req.Copies},
		frsecurity.AdmissionLimits{MaxBytes: 100 << 20, MaxCopies: 1000},
	); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "resource_policy_rejected", "resource policy rejected ingest")
		return
	}
	path, err := s.artifactPath(req.ArtifactSHA256)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_artifact_digest", err.Error())
		return
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() != req.ArtifactBytes {
		writeError(w, http.StatusUnprocessableEntity, "artifact_not_staged", "staged artifact is missing or size does not match")
		return
	}

	acceptedAt := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.state.Apply(frstate.Command{
		Type:                "ingest-artifact",
		AggregateID:         req.AggregateID,
		IdempotencyKey:      idempotencyKey,
		SemanticFingerprint: req.SemanticFingerprint,
		Metadata: map[string]string{
			"inbox_version":   "1",
			"accepted_at":      acceptedAt,
			"artifact_sha256":  req.ArtifactSHA256,
			"artifact_bytes":   strconv.FormatInt(req.ArtifactBytes, 10),
			"media_type":       req.MediaType,
			"copies":           strconv.FormatInt(req.Copies, 10),
			"substrate":        req.Substrate,
			"substrate_job_id": req.SubstrateJobID,
		},
	}, func(current frstate.Snapshot, _ frstate.Command) (frstate.TransitionResult, error) {
		if current.Generation != 0 {
			return frstate.TransitionResult{State: current.State, Changed: false}, nil
		}
		return frstate.TransitionResult{
			State:   "accepted",
			Changed: true,
			Effects: []frstate.EffectSpec{{
				Kind:          "artifact-accepted",
				PayloadDigest: "sha256:" + req.ArtifactSHA256,
			}},
		}, nil
	})
	if errors.Is(err, frstate.ErrIdempotencyConflict) {
		writeError(w, http.StatusConflict, "idempotency_conflict", "idempotency conflict")
		return
	}
	if err != nil {
		writeError(w, http.StatusConflict, "state_transition_rejected", "state transition rejected")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"job_id":       frinbox.JobID(req.AggregateID),
		"aggregate_id": req.AggregateID,
		"state":        result.Snapshot.State,
		"generation":   result.Snapshot.Generation,
		"replayed":     result.Replayed,
		"effects":      result.Effects,
	})
}

func (s *Server) findJob(jobID string) (frinbox.Record, bool, error) {
	jobs, err := frinbox.Project(s.state.Records())
	if err != nil {
		return frinbox.Record{}, false, err
	}
	for _, record := range jobs {
		if record.JobID == jobID {
			return record, true, nil
		}
	}
	return frinbox.Record{}, false, nil
}

func (s *Server) artifactPath(digest string) (string, error) {
	if len(digest) != 64 || strings.ToLower(digest) != digest {
		return "", errors.New("artifact digest must be 64 lowercase hexadecimal characters")
	}
	decoded, err := hex.DecodeString(digest)
	if err != nil || len(decoded) != 32 {
		return "", errors.New("artifact digest must be 64 lowercase hexadecimal characters")
	}
	return filepath.Join(s.artifactStore, "sha256", digest[:2], digest), nil
}

func encodeCursor(offset int) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.Itoa(offset)))
}

func decodeCursor(cursor string) (int, error) {
	if cursor == "" {
		return 0, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, err
	}
	offset, err := strconv.Atoi(string(raw))
	if err != nil || offset < 0 {
		return 0, errors.New("invalid cursor")
	}
	return offset, nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}
