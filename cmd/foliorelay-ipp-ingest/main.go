package main

import (
	"bytes"
	"crypto/sha256"
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
)

const defaultMaxBytes int64 = 100 << 20

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

func main() {
	if len(os.Args) != 2 {
		fail("expected exactly one spool file argument")
	}
	source := os.Args[1]
	jobID, err := boundedEnv("IPP_JOB_ID", 64, true)
	if err != nil {
		fail(err.Error())
	}
	jobUUID, err := boundedEnv("IPP_JOB_UUID", 256, false)
	if err != nil {
		fail(err.Error())
	}
	mediaType, err := boundedEnv("CONTENT_TYPE", 128, true)
	if err != nil {
		fail(err.Error())
	}
	if strings.ContainsAny(mediaType, "\r\n\x00") {
		fail("invalid content type")
	}
	copies, err := positiveIntEnv("IPP_COPIES", 1, 10_000_000)
	if err != nil {
		fail(err.Error())
	}

	maxBytes := defaultMaxBytes
	if raw := os.Getenv("FOLIORELAY_MAX_INGRESS_BYTES"); raw != "" {
		value, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || value <= 0 {
			fail("invalid FOLIORELAY_MAX_INGRESS_BYTES")
		}
		maxBytes = value
	}

	store := os.Getenv("FOLIORELAY_INGRESS_STORE")
	if store == "" {
		fail("FOLIORELAY_INGRESS_STORE is required")
	}

	digest, size, err := storeBlob(source, store, maxBytes)
	if err != nil {
		fail(err.Error())
	}

	stableJob := jobUUID
	if stableJob == "" {
		stableJob = "job-" + jobID
	}
	aggregate := "ipp/ippeveprinter/" + stableJob
	key := aggregate

	fingerprintInput := strings.Join([]string{
		"v1", aggregate, digest, strconv.FormatInt(size, 10), mediaType,
		strconv.FormatInt(copies, 10),
	}, "\x00")
	fpSum := sha256.Sum256([]byte(fingerprintInput))

	req := ingestRequest{
		AggregateID:         aggregate,
		IdempotencyKey:      key,
		SemanticFingerprint: "sha256:" + hex.EncodeToString(fpSum[:]),
		ArtifactSHA256:      digest,
		ArtifactBytes:       size,
		MediaType:           mediaType,
		Substrate:           "ippeveprinter",
		SubstrateJobID:      jobID,
		Copies:              copies,
	}
	payload, _ := json.Marshal(req)

	url := os.Getenv("FOLIORELAY_STATE_URL")
	if url == "" {
		url = "http://127.0.0.1:18080"
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(url+"/v1/ingest", "application/json", bytes.NewReader(payload))
	if err != nil {
		fail("state authority unavailable")
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode/100 != 2 {
		fail(fmt.Sprintf("state authority rejected ingest: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body))))
	}
	fmt.Fprintf(os.Stderr, "INFO: FolioRelay accepted artifact sha256:%s (%d bytes)\n", digest, size)
}

func boundedEnv(name string, max int, required bool) (string, error) {
	value := os.Getenv(name)
	if required && value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	if len(value) > max {
		return "", fmt.Errorf("%s exceeds limit", name)
	}
	for _, r := range value {
		if r == 0 || r == '\r' || r == '\n' || r < 0x20 || r == 0x7f {
			return "", fmt.Errorf("%s contains control characters", name)
		}
	}
	return value, nil
}

func positiveIntEnv(name string, fallback, max int64) (int64, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 1 || value > max {
		return 0, fmt.Errorf("%s is outside accepted bounds", name)
	}
	return value, nil
}

func storeBlob(source, store string, maxBytes int64) (string, int64, error) {
	in, err := os.Open(source)
	if err != nil {
		return "", 0, fmt.Errorf("open spool file: %w", err)
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return "", 0, fmt.Errorf("stat spool file: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > maxBytes {
		return "", 0, errors.New("spool file rejected by size/type policy")
	}

	h := sha256.New()
	tmpRoot := filepath.Join(store, ".staging")
	if err := os.MkdirAll(tmpRoot, 0o700); err != nil {
		return "", 0, fmt.Errorf("create staging directory: %w", err)
	}
	tmp, err := os.CreateTemp(tmpRoot, "ingest-*")
	if err != nil {
		return "", 0, fmt.Errorf("create staging file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		_ = tmp.Close()
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()

	limited := io.LimitReader(in, maxBytes+1)
	written, err := io.Copy(io.MultiWriter(tmp, h), limited)
	if err != nil {
		return "", 0, fmt.Errorf("stage artifact: %w", err)
	}
	if written > maxBytes {
		return "", 0, errors.New("artifact exceeds configured ingress limit")
	}
	if err := tmp.Sync(); err != nil {
		return "", 0, fmt.Errorf("fsync staged artifact: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", 0, fmt.Errorf("close staged artifact: %w", err)
	}

	digest := hex.EncodeToString(h.Sum(nil))
	targetDir := filepath.Join(store, "sha256", digest[:2])
	if err := os.MkdirAll(targetDir, 0o700); err != nil {
		return "", 0, fmt.Errorf("create object directory: %w", err)
	}
	target := filepath.Join(targetDir, digest)
	if err := os.Link(tmpName, target); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return "", 0, fmt.Errorf("commit content-addressed artifact: %w", err)
		}
	} else {
		if dir, openErr := os.Open(targetDir); openErr == nil {
			_ = dir.Sync()
			_ = dir.Close()
		}
	}
	_ = os.Remove(tmpName)
	cleanup = false
	return digest, written, nil
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, "ERROR:", message)
	os.Exit(1)
}
