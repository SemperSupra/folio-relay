// Package cloudcopy plans one authorized, immutable-artifact cloud delivery.
// Planning has no provider credentials, network I/O, or rclone execution.
package cloudcopy

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/SemperSupra/folio-relay/internal/export"
)

const (
	maxFilename  = 180
	maxObjectKey = 512
)

var (
	ErrInvalidPlan   = errors.New("invalid cloud copy plan")
	profileIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)
	actionIDPattern  = regexp.MustCompile(`^[a-zA-Z0-9_-]{8,80}$`)
	digestPattern    = regexp.MustCompile(`^[0-9a-f]{64}$`)
	jobIDPattern     = regexp.MustCompile(`^job-[0-9a-f]{64}$`)
)

// Profile is a non-secret reference to an independently enrolled destination.
// The adapter must resolve this ID to one fixed, scoped rclone configuration.
type Profile struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
}

// validatePlan rejects altered or raw rclone destinations before an intent is
// durably admitted. It does not claim that a provider can create without
// replacing an existing object; that needs separate adapter qualification.
func validatePlan(p Plan) error {
	if !actionIDPattern.MatchString(p.ActionID) || !jobIDPattern.MatchString(p.JobID) ||
		!profileIDPattern.MatchString(p.ProfileID) || !digestPattern.MatchString(p.ArtifactSHA256) ||
		p.ArtifactBytes < 0 || p.ArtifactBytes > 1<<30 || len(p.ObjectKey) > maxObjectKey || len(p.MediaType) > 128 {
		return ErrInvalidPlan
	}
	switch p.Provider {
	case "google_drive", "onedrive", "icloud_drive", "nextcloud", "s3":
	default:
		return ErrInvalidPlan
	}
	if p.RemoteName != "fr_"+p.ProfileID || p.RclonePath != p.RemoteName+":"+p.ObjectKey ||
		!strings.HasPrefix(p.ObjectKey, "foliorelay/") || path.Clean(p.ObjectKey) != p.ObjectKey ||
		strings.ContainsAny(p.ObjectKey, "\\:") || len(p.ObjectKey) == 0 {
		return ErrInvalidPlan
	}
	for _, component := range strings.Split(p.ObjectKey, "/") {
		if component == "" || component == "." || component == ".." || strings.HasPrefix(component, ".") ||
			strings.HasSuffix(component, ".") || strings.HasSuffix(component, " ") || len(component) > 180 {
			return ErrInvalidPlan
		}
		for _, r := range component {
			if unicode.IsControl(r) || strings.ContainsRune(`<>"|?*`, r) {
				return ErrInvalidPlan
			}
		}
	}
	filename := path.Base(p.ObjectKey)
	suffix := sha256.Sum256([]byte("foliorelay-cloud-copy/v1\x00" + p.ActionID))
	if !strings.HasSuffix(filename, "-"+hex.EncodeToString(suffix[:])+path.Ext(filename)) ||
		path.Ext(filename) != extensionForMedia(p.MediaType) {
		return ErrInvalidPlan
	}
	return nil
}

func extensionForMedia(media string) string {
	switch strings.ToLower(strings.TrimSpace(strings.SplitN(media, ";", 2)[0])) {
	case "application/pdf":
		return ".pdf"
	case "image/urf":
		return ".urf"
	case "image/pwg-raster":
		return ".pwg"
	case "application/postscript":
		return ".ps"
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "text/plain":
		return ".txt"
	default:
		return ".bin"
	}
}

// Plan is inert. The remote name is derived from a trusted profile ID, and
// the object key is generated rather than accepted as a raw rclone expression.
type Plan struct {
	ActionID       string `json:"action_id"`
	JobID          string `json:"job_id"`
	ProfileID      string `json:"profile_id"`
	Provider       string `json:"provider"`
	ArtifactSHA256 string `json:"artifact_sha256"`
	ArtifactBytes  int64  `json:"artifact_bytes"`
	MediaType      string `json:"media_type"`
	RemoteName     string `json:"remote_name"`
	ObjectKey      string `json:"object_key"`
	RclonePath     string `json:"rclone_path"`
}

// MakePlan uses the existing export naming policy, then adds an action-specific
// suffix. A second explicit delivery of the same job gets a distinct key.
// Destination existence/no-clobber still needs backend-specific qualification.
func MakePlan(profile Profile, actionID string, defaults export.Defaults, overrides export.Overrides, job export.Job, artifact export.Artifact) (Plan, error) {
	if !profileIDPattern.MatchString(profile.ID) || !actionIDPattern.MatchString(actionID) ||
		!digestPattern.MatchString(artifact.SHA256) || artifact.Bytes < 0 || artifact.Bytes > 1<<30 {
		return Plan{}, ErrInvalidPlan
	}
	switch profile.Provider {
	case "google_drive", "onedrive", "icloud_drive", "nextcloud", "s3":
	default:
		return Plan{}, ErrInvalidPlan
	}
	name, err := export.MakePlan(defaults, overrides, job)
	if err != nil {
		return Plan{}, err
	}
	// Preserve the MIME-derived extension and keep the full action digest so
	// retries are stable and independently authorized actions get new keys.
	ext := path.Ext(name.Filename)
	stem := strings.TrimSuffix(name.Filename, ext)
	suffix := sha256.Sum256([]byte("foliorelay-cloud-copy/v1\x00" + actionID))
	actionTag := "-" + hex.EncodeToString(suffix[:])
	maxStem := maxFilename - len(actionTag) - len(ext)
	if maxStem < 1 {
		return Plan{}, ErrInvalidPlan
	}
	if len(stem) > maxStem {
		cut := 0
		for _, r := range stem {
			if cut+utf8.RuneLen(r) > maxStem {
				break
			}
			cut += utf8.RuneLen(r)
		}
		stem = stem[:cut]
	}
	stem = strings.TrimRight(stem, "-_. ")
	if stem == "" {
		return Plan{}, ErrInvalidPlan
	}
	filename := stem + actionTag + ext
	key := path.Join("foliorelay", name.Folder, filename)
	if len(key) > maxObjectKey {
		return Plan{}, ErrInvalidPlan
	}
	remote := "fr_" + profile.ID
	return Plan{
		ActionID: actionID, JobID: job.JobID, ProfileID: profile.ID, Provider: profile.Provider,
		ArtifactSHA256: artifact.SHA256, ArtifactBytes: artifact.Bytes,
		MediaType:  name.MediaType,
		RemoteName: remote, ObjectKey: key, RclonePath: remote + ":" + key,
	}, nil
}
