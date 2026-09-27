package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"sync/atomic"

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
}

type stats struct {
	Accepted  atomic.Uint64
	Replayed  atomic.Uint64
	Conflicts atomic.Uint64
}

func main() {
	listen := flag.String("listen", "127.0.0.1:18080", "listen address")
	flag.Parse()

	var engine frstate.Engine
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
			req.ArtifactBytes < 0 || req.MediaType == "" || req.Substrate == "" {
			http.Error(w, "missing required field", http.StatusBadRequest)
			return
		}

		result, err := engine.Apply(frstate.Command{
			Type:                "ingest-artifact",
			AggregateID:         req.AggregateID,
			IdempotencyKey:      req.IdempotencyKey,
			SemanticFingerprint: req.SemanticFingerprint,
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
	log.Printf("state fixture listening on %s", *listen)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(fmt.Errorf("serve: %w", err))
	}
}
