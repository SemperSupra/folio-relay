package control

import (
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

	frinbox "github.com/SemperSupra/folio-relay/internal/inbox"
	frprinter "github.com/SemperSupra/folio-relay/internal/printer"
	frsecurity "github.com/SemperSupra/folio-relay/internal/security"
	frstate "github.com/SemperSupra/folio-relay/internal/state"
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
	mux.Handle("GET /api/v1/status", s.requireAuth(http.HandlerFunc(s.handleStatus)))
	mux.Handle("GET /api/v1/printer", s.requireAuth(http.HandlerFunc(s.handlePrinter)))
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
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if len(got) != len(s.token) ||
			subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
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
			"openapi":   "/openapi.json",
			"status":    "/api/v1/status",
			"jobs":      "/api/v1/jobs",
			"self_test": "/api/v1/self-test",
		},
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
