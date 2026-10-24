package cloudcopy

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/SemperSupra/folio-relay/internal/export"
)

func fixture() (Profile, export.Defaults, export.Job, export.Artifact) {
	sum := sha256.Sum256([]byte("synthetic artifact"))
	return Profile{ID: "home_drive", Provider: "google_drive"},
		export.Defaults{Folder: "Records/2026", FilenameTemplate: "{job_name}-{timestamp}-{job_id}"},
		export.Job{JobID: "job-" + strings.Repeat("a", 64), SourceTitle: "Quarterly report.pdf", AcceptedAt: time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC), MediaType: "application/pdf"},
		export.Artifact{SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len("synthetic artifact"))}
}

func TestOneActionHasStableDistinctRemoteKey(t *testing.T) {
	profile, defaults, job, artifact := fixture()
	a, err := MakePlan(profile, "action_0001", defaults, export.Overrides{}, job, artifact)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := MakePlan(profile, "action_0001", defaults, export.Overrides{}, job, artifact)
	if err != nil || replay != a {
		t.Fatalf("replay changed: %#v %v", replay, err)
	}
	b, err := MakePlan(profile, "action_0002", defaults, export.Overrides{}, job, artifact)
	if err != nil {
		t.Fatal(err)
	}
	if a.ObjectKey == b.ObjectKey {
		t.Fatal("new action reused remote key")
	}
	if !strings.HasPrefix(a.RclonePath, "fr_home_drive:foliorelay/Records/2026/") || !strings.HasSuffix(a.RclonePath, ".pdf") {
		t.Fatalf("unexpected destination %q", a.RclonePath)
	}
	if a.ArtifactSHA256 != artifact.SHA256 || a.ArtifactBytes != artifact.Bytes || a.JobID != job.JobID {
		t.Fatal("artifact lineage lost")
	}
}

func TestPlanRejectsRawRemoteAndTraversal(t *testing.T) {
	profile, defaults, job, artifact := fixture()
	for _, id := range []string{"../other", "drive:other", "a/b", "", strings.Repeat("a", 65)} {
		profile.ID = id
		if _, err := MakePlan(profile, "action_0001", defaults, export.Overrides{}, job, artifact); !errors.Is(err, ErrInvalidPlan) {
			t.Errorf("accepted profile %q: %v", id, err)
		}
	}
	profile.ID = "home_drive"
	for _, folder := range []string{"../outside", "/absolute", "Records\\other", "bad//folder"} {
		if _, err := MakePlan(profile, "action_0001", defaults, export.Overrides{Folder: folder}, job, artifact); err == nil {
			t.Errorf("accepted folder %q", folder)
		}
	}
	profile.Provider = "unreviewed"
	if _, err := MakePlan(profile, "action_0001", defaults, export.Overrides{}, job, artifact); !errors.Is(err, ErrInvalidPlan) {
		t.Fatal(err)
	}
}

func TestLongNameRetainsExtensionAndBoundedKey(t *testing.T) {
	profile, defaults, job, artifact := fixture()
	job.SourceTitle = strings.Repeat("A", 150)
	plan, err := MakePlan(profile, "action_0001", defaults, export.Overrides{}, job, artifact)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(plan.ObjectKey, ".pdf") || len(plan.ObjectKey) > maxObjectKey {
		t.Fatalf("invalid key %q", plan.ObjectKey)
	}
	defaults.FilenameTemplate = "{job_id}-X" + strings.Repeat("ä", 40)
	plan, err = MakePlan(profile, "action_0001", defaults, export.Overrides{}, job, artifact)
	if err != nil {
		t.Fatal(err)
	}
	if !utf8.ValidString(plan.ObjectKey) {
		t.Fatal("truncation split a UTF-8 character")
	}
}
