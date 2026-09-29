package inbox

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	frstate "github.com/SemperSupra/folio-relay/internal/state"
)

const metadataVersion = "1"

type Record struct {
	JobID          string    `json:"job_id"`
	AggregateID    string    `json:"aggregate_id"`
	AcceptedAt     time.Time `json:"accepted_at"`
	State          string    `json:"state"`
	ArtifactSHA256 string    `json:"artifact_sha256"`
	ArtifactBytes  int64     `json:"artifact_bytes"`
	MediaType      string    `json:"media_type"`
	Copies         int64     `json:"copies"`
	Substrate      string    `json:"substrate"`
	SubstrateJobID string    `json:"substrate_job_id"`
}

// Project derives the current Inbox from authoritative commit records.
//
// Records created before Inbox metadata existed are intentionally ignored.
// Once a command declares inbox_version=1, malformed metadata is an error:
// silently projecting a partial/current job would make the UI/API disagree
// with durable authority.
func Project(records []frstate.CommitRecord) ([]Record, error) {
	out := make([]Record, 0, len(records))
	for i := len(records) - 1; i >= 0; i-- {
		record := records[i]
		if record.Command.Type != "ingest-artifact" ||
			!record.Changed ||
			record.Result.Snapshot.State != "accepted" {
			continue
		}
		metadata := record.Command.Metadata
		if metadata == nil || metadata["inbox_version"] == "" {
			continue
		}
		if metadata["inbox_version"] != metadataVersion {
			return nil, fmt.Errorf("inbox projection: unsupported metadata version %q for %s",
				metadata["inbox_version"], record.Command.AggregateID)
		}

		acceptedAt, err := time.Parse(time.RFC3339Nano, metadata["accepted_at"])
		if err != nil {
			return nil, fmt.Errorf("inbox projection: invalid accepted_at for %s: %w",
				record.Command.AggregateID, err)
		}
		artifactBytes, err := strconv.ParseInt(metadata["artifact_bytes"], 10, 64)
		if err != nil || artifactBytes < 0 {
			return nil, fmt.Errorf("inbox projection: invalid artifact_bytes for %s",
				record.Command.AggregateID)
		}
		copies, err := strconv.ParseInt(metadata["copies"], 10, 64)
		if err != nil || copies < 1 || copies > 1000 {
			return nil, fmt.Errorf("inbox projection: invalid copies for %s",
				record.Command.AggregateID)
		}
		artifactSHA := metadata["artifact_sha256"]
		decoded, err := hex.DecodeString(artifactSHA)
		if err != nil || len(decoded) != 32 {
			return nil, fmt.Errorf("inbox projection: invalid artifact_sha256 for %s",
				record.Command.AggregateID)
		}
		if metadata["media_type"] == "" || metadata["substrate"] == "" ||
			metadata["substrate_job_id"] == "" {
			return nil, fmt.Errorf("inbox projection: missing required metadata for %s",
				record.Command.AggregateID)
		}

		out = append(out, Record{
			JobID:          record.Command.AggregateID,
			AggregateID:    record.Command.AggregateID,
			AcceptedAt:     acceptedAt,
			State:          "accepted",
			ArtifactSHA256: artifactSHA,
			ArtifactBytes:  artifactBytes,
			MediaType:      metadata["media_type"],
			Copies:         copies,
			Substrate:      metadata["substrate"],
			SubstrateJobID: metadata["substrate_job_id"],
		})
	}
	return out, nil
}
