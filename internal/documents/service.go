// Package documents owns reversible annotations and Inbox review state for
// accepted jobs. It never creates intake records or changes canonical bytes.
package documents

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/SemperSupra/folio-relay/internal/inbox"
	"github.com/SemperSupra/folio-relay/internal/state"
)

var (
	ErrInvalid      = errors.New("invalid document command")
	ErrNotFound     = errors.New("document not found")
	ErrUnauthorized = errors.New("document action unauthorized")
	ErrNoChange     = errors.New("document action has no change")
	jobPattern      = regexp.MustCompile(`^job-[0-9a-f]{64}$`)
	actionPattern   = regexp.MustCompile(`^[A-Za-z0-9_-]{8,80}$`)
)

// Principal must be obtained from a separately authenticated transport.
// A shared management token does not identify a document owner.
type Principal struct {
	ID   string `json:"id"`
	Kind string `json:"kind"` // human, automation, agent
}

// Authorizer is mandatory. No default allow-all policy is provided.
type Authorizer interface {
	Authorize(Principal, inbox.Record, string) error
}

type Annotation struct {
	Label    string   `json:"label"`
	Tags     []string `json:"tags"`
	Note     string   `json:"note"`
	Archived bool     `json:"archived"`
}

type Document struct {
	Job        inbox.Record `json:"job"`
	Annotation Annotation   `json:"annotation"`
	Revision   uint64       `json:"revision"`
}

// Command replaces the editable fields or changes Inbox review state. The
// stable job ID and original accepted-artifact fields are never writable.
type Command struct {
	JobID            string     `json:"job_id"`
	ActionID         string     `json:"action_id"`
	Actor            Principal  `json:"actor"`
	ExpectedRevision uint64     `json:"expected_revision"`
	Kind             string     `json:"kind"` // set_metadata, archive, restore
	Metadata         Annotation `json:"metadata,omitempty"`
}

type Receipt struct {
	JobID    string `json:"job_id"`
	ActionID string `json:"action_id"`
	Revision uint64 `json:"revision"`
	State    string `json:"state"`
	Replayed bool   `json:"replayed"`
}

type Service struct {
	durable *state.DurableEngine
	auth    Authorizer
}

func NewService(d *state.DurableEngine, auth Authorizer) (*Service, error) {
	if d == nil || auth == nil {
		return nil, ErrInvalid
	}
	return &Service{durable: d, auth: auth}, nil
}

func validPrincipal(p Principal) bool {
	if p.ID == "" || p.ID != strings.TrimSpace(p.ID) || len(p.ID) > 128 || !utf8.ValidString(p.ID) {
		return false
	}
	for _, r := range p.ID {
		if unicode.IsControl(r) {
			return false
		}
	}
	switch p.Kind {
	case "human", "automation", "agent":
		return true
	}
	return false
}

func validText(s string, max int, newline bool) bool {
	if len(s) > max || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) && !(newline && r == '\n') {
			return false
		}
	}
	return true
}

func validAnnotation(a Annotation) bool {
	if !validText(a.Label, 180, false) || !validText(a.Note, 2000, true) || len(a.Tags) > 12 {
		return false
	}
	seen := make(map[string]bool, len(a.Tags))
	for _, tag := range a.Tags {
		if tag == "" || tag != strings.TrimSpace(tag) || !validText(tag, 64, false) || seen[tag] {
			return false
		}
		seen[tag] = true
	}
	return true
}

func validCommand(c Command) bool {
	if !jobPattern.MatchString(c.JobID) || !actionPattern.MatchString(c.ActionID) || !validPrincipal(c.Actor) {
		return false
	}
	switch c.Kind {
	case "set_metadata":
		return !c.Metadata.Archived && validAnnotation(c.Metadata)
	case "archive", "restore":
		return c.Metadata.Label == "" && c.Metadata.Note == "" && len(c.Metadata.Tags) == 0 && !c.Metadata.Archived
	}
	return false
}

func (s *Service) findJob(id string, records []state.CommitRecord) (inbox.Record, error) {
	jobs, err := inbox.Project(records)
	if err != nil {
		return inbox.Record{}, err
	}
	for _, j := range jobs {
		if j.JobID == id {
			return j, nil
		}
	}
	return inbox.Record{}, ErrNotFound
}

func (s *Service) view(id string) (Document, error) {
	if s == nil || s.durable == nil || !jobPattern.MatchString(id) {
		return Document{}, ErrInvalid
	}
	agg := "document/" + id
	snap := s.durable.Snapshot(agg)
	records := s.durable.Records()
	job, err := s.findJob(id, records)
	if err != nil {
		return Document{}, err
	}
	doc := Document{Job: job, Annotation: Annotation{Tags: []string{}}, Revision: snap.Generation}
	if snap.Generation == 0 {
		return doc, nil
	}
	for i := len(records) - 1; i >= 0; i-- {
		r := records[i]
		if r.Command.AggregateID != agg || r.Result.Snapshot.Generation != snap.Generation || r.Command.Type != "document-change" {
			continue
		}
		if err := json.Unmarshal([]byte(r.Command.Metadata["next"]), &doc.Annotation); err != nil {
			return Document{}, err
		}
		if r.Result.Snapshot.State == "archived" != doc.Annotation.Archived {
			return Document{}, ErrInvalid
		}
		return doc, nil
	}
	return Document{}, ErrInvalid
}

// Get returns the durable document projection under a caller-supplied policy.
func (s *Service) Get(actor Principal, id string) (Document, error) {
	if !validPrincipal(actor) {
		return Document{}, ErrInvalid
	}
	doc, err := s.view(id)
	if err != nil {
		return Document{}, err
	}
	if err := s.auth.Authorize(actor, doc.Job, "read"); err != nil {
		return Document{}, ErrUnauthorized
	}
	return doc, nil
}

// Apply commits a reversible user annotation/review action. Replayed commands
// return their original revision, even if later actions changed the document.
func (s *Service) Apply(c Command) (Receipt, error) {
	if !validCommand(c) {
		return Receipt{}, ErrInvalid
	}
	current, err := s.view(c.JobID)
	if err != nil {
		return Receipt{}, err
	}
	if err := s.auth.Authorize(c.Actor, current.Job, c.Kind); err != nil {
		return Receipt{}, ErrUnauthorized
	}
	next := current.Annotation
	switch c.Kind {
	case "set_metadata":
		next.Label, next.Note = c.Metadata.Label, c.Metadata.Note
		next.Tags = slices.Clone(c.Metadata.Tags)
	case "archive":
		next.Archived = true
	case "restore":
		next.Archived = false
	}
	request, err := json.Marshal(c)
	if err != nil {
		return Receipt{}, err
	}
	fingerprint := sha256.Sum256(request)
	before, err := json.Marshal(current.Annotation)
	if err != nil {
		return Receipt{}, err
	}
	after, err := json.Marshal(next)
	if err != nil {
		return Receipt{}, err
	}
	result, err := s.durable.Apply(state.Command{
		Type: "document-change", AggregateID: "document/" + c.JobID,
		IdempotencyKey: "action/" + c.ActionID, SemanticFingerprint: hex.EncodeToString(fingerprint[:]),
		ExpectedGeneration: &c.ExpectedRevision,
		Metadata: map[string]string{
			"actor_id": c.Actor.ID, "actor_kind": c.Actor.Kind, "action": c.Kind,
			"job_id": c.JobID, "source_sha256": current.Job.ArtifactSHA256,
			"previous": string(before), "next": string(after),
		},
	}, func(old state.Snapshot, _ state.Command) (state.TransitionResult, error) {
		if old.Generation != current.Revision {
			return state.TransitionResult{}, state.ErrGenerationConflict
		}
		if current.Annotation.Archived == next.Archived && current.Annotation.Label == next.Label &&
			current.Annotation.Note == next.Note && slices.Equal(current.Annotation.Tags, next.Tags) {
			return state.TransitionResult{}, ErrNoChange
		}
		status := "active"
		if next.Archived {
			status = "archived"
		}
		return state.TransitionResult{State: status, Changed: true}, nil
	})
	if err != nil {
		return Receipt{}, err
	}
	return Receipt{JobID: c.JobID, ActionID: c.ActionID, Revision: result.Snapshot.Generation,
		State: result.Snapshot.State, Replayed: result.Replayed}, nil
}
