package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	frsecurity "github.com/SemperSupra/folio-relay/internal/security"
	frstate "github.com/SemperSupra/folio-relay/internal/state"
)

type ingestRequest struct {
	AggregateID         string `json:"aggregate_id"`
	IdempotencyKey      string `json:"idempotency_key"`
	SemanticFingerprint string `json:"semantic_fingerprint"`
	ArtifactSHA256      string `json:"artifact_sha256"`
	ArtifactBytes       int64  `json:"artifact_bytes"`
	MediaType           string `json:"media_type"`
	Substrate           string `json:"substrate"`
	SubstrateJobID      string `json:"substrate_job_id"`
	Copies              int64  `json:"copies"`
}

type stats struct {
	Accepted  atomic.Uint64
	Replayed  atomic.Uint64
	Conflicts atomic.Uint64
	Rejected  atomic.Uint64
}

type applyEngine interface {
	Apply(frstate.Command, frstate.Transition) (frstate.ApplyResult, error)
}

func main() {
	listen := flag.String("listen", "127.0.0.1:18080", "listen address")
	journalPath := flag.String("journal", "", "optional durable journal path")
	flag.Parse()

	var engine applyEngine
	var closeEngine func() error
	if *journalPath != "" {
		durable, err := frstate.OpenDurableEngine(*journalPath)
		if err != nil {
			log.Fatal(fmt.Errorf("open durable state: %w", err))
		}
		engine = durable
		closeEngine = durable.Close
		defer func() {
			if err := closeEngine(); err != nil {
				log.Printf("close durable state: %v", err)
			}
		}()
	} else {
		engine = &frstate.Engine{}
	}

	var counters stats
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/ingest", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		defer r.Body.Close()

		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
		dec.DisallowUnknownFields()
		var req ingestRequest
		if err := dec.Decode(&req); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if req.AggregateID == "" || req.IdempotencyKey == "" ||
			req.SemanticFingerprint == "" || req.ArtifactSHA256 == "" ||
			req.ArtifactBytes < 0 || req.MediaType == "" || req.Substrate == "" ||
			req.Copies < 1 {
			http.Error(w, "missing or invalid required field", http.StatusBadRequest)
			return
		}
		if err := frsecurity.ValidateAdmission(
			frsecurity.AdmissionRequest{Bytes: req.ArtifactBytes, Copies: req.Copies},
			frsecurity.AdmissionLimits{MaxBytes: 100 << 20, MaxCopies: 1000},
		); err != nil {
			counters.Rejected.Add(1)
			http.Error(w, "resource policy rejected ingest", http.StatusUnprocessableEntity)
			return
		}

		acceptedAt := time.Now().UTC().Format(time.RFC3339Nano)
		result, err := engine.Apply(frstate.Command{
			Type:                "ingest-artifact",
			AggregateID:         req.AggregateID,
			IdempotencyKey:      req.IdempotencyKey,
			SemanticFingerprint: req.SemanticFingerprint,
			Metadata: map[string]string{
				"inbox_version":    "1",
				"accepted_at":       acceptedAt,
				"artifact_sha256":   req.ArtifactSHA256,
				"artifact_bytes":    strconv.FormatInt(req.ArtifactBytes, 10),
				"media_type":        req.MediaType,
				"copies":            strconv.FormatInt(req.Copies, 10),
				"substrate":         req.Substrate,
				"substrate_job_id":  req.SubstrateJobID,
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
		if err != nil {
			if errors.Is(err, frstate.ErrIdempotencyConflict) {
				counters.Conflicts.Add(1)
				http.Error(w, "idempotency conflict", http.StatusConflict)
				return
			}
			http.Error(w, "state transition rejected", http.StatusConflict)
			return
		}

		if result.Replayed {
			counters.Replayed.Add(1)
		} else {
			counters.Accepted.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"aggregate_id": req.AggregateID,
			"state":        result.Snapshot.State,
			"generation":   result.Snapshot.Generation,
			"replayed":     result.Replayed,
			"effects":      result.Effects,
		})
	})

	mux.HandleFunc("/v1/stats", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]uint64{
			"accepted":  counters.Accepted.Load(),
			"replayed":  counters.Replayed.Load(),
			"conflicts": counters.Conflicts.Load(),
			"rejected":  counters.Rejected.Load(),
		})
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	server := &http.Server{
		Addr:              *listen,
		Handler:           mux,
		ReadHeaderTimeout: 5 * 1e9,
	}
	if *journalPath != "" {
		log.Printf("state fixture listening on %s with durable journal %s", *listen, *journalPath)
	} else {
		log.Printf("state fixture listening on %s with ephemeral state", *listen)
	}
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(fmt.Errorf("serve: %w", err))
	}
}
