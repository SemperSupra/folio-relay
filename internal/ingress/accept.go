package ingress

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/SemperSupra/folio-relay/internal/state"
)

var (
	ErrInvalidSource = errors.New("invalid ingress source identity")
	ErrInvalidPath   = errors.New("invalid ingress artifact path")
)

type Source struct {
	Substrate  string
	InstanceID string
	JobID      string
}

type Receipt struct {
	Source         Source
	ArtifactDigest string
	Bytes          int64
	Command        state.Command
}

func PrepareFile(source Source, path string) (Receipt, error) {
	if strings.TrimSpace(source.Substrate) == "" ||
		strings.TrimSpace(source.InstanceID) == "" ||
		strings.TrimSpace(source.JobID) == "" {
		return Receipt{}, ErrInvalidSource
	}
	if path == "" {
		return Receipt{}, ErrInvalidPath
	}

	f, err := os.Open(path)
	if err != nil {
		return Receipt{}, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return Receipt{}, err
	}
	if !info.Mode().IsRegular() {
		return Receipt{}, ErrInvalidPath
	}

	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return Receipt{}, err
	}
	digest := "sha256:" + hex.EncodeToString(h.Sum(nil))

	semantic := sha256.New()
	fmt.Fprintf(semantic, "accept-ingress\x00%s\x00%s\x00%s\x00%s\x00%d",
		source.Substrate, source.InstanceID, source.JobID, digest, n)
	fingerprint := "sha256:" + hex.EncodeToString(semantic.Sum(nil))

	identity := source.Substrate + ":" + source.InstanceID + ":" + source.JobID
	return Receipt{
		Source: source,
		ArtifactDigest: digest,
		Bytes: n,
		Command: state.Command{
			Type: "accept-ingress",
			AggregateID: "job:" + identity,
			IdempotencyKey: "ingress:" + identity,
			SemanticFingerprint: fingerprint,
		},
	}, nil
}
