// Package export plans names for managed output copies. It deliberately does
// not write files, configure SMB, or grant access to canonical artifacts.
package export

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"
	"unicode"
)

const (
	DefaultTemplate = "{job_name}-{timestamp}-{job_id}"
	maxFilename     = 180
	maxFolder       = 220
)

var jobIDPattern = regexp.MustCompile(`^job-[0-9a-f]{64}$`)

// Defaults are printer-wide settings. Folder is relative to the dedicated
// export root, never the canonical artifact store. "" and "." mean its root.
type Defaults struct {
	Folder           string
	FilenameTemplate string
}

// Overrides must come only from an authenticated and authorized job action.
// Empty values mean no override; no printer protocol is trusted to supply them.
type Overrides struct {
	Folder           string
	FilenameTemplate string
}

type Job struct {
	JobID       string
	SourceTitle string
	Submitter   string
	AcceptedAt  time.Time
	MediaType   string
}

type Plan struct {
	RelativePath string
	Folder       string
	Filename     string
	MediaType    string
}

// MakePlan creates an inert, platform-safe relative path. The filesystem
// worker must separately use a confined root, exclusive create, atomic
// publication, digest verification and explicit export reconciliation.
func MakePlan(defaults Defaults, override Overrides, job Job) (Plan, error) {
	if !jobIDPattern.MatchString(job.JobID) || job.AcceptedAt.IsZero() {
		return Plan{}, errors.New("export requires a durable job ID and acceptance time")
	}
	folder := defaults.Folder
	if override.Folder != "" {
		folder = override.Folder
	}
	normalizedFolder, err := validateFolder(folder)
	if err != nil {
		return Plan{}, err
	}

	template := defaults.FilenameTemplate
	if override.FilenameTemplate != "" {
		template = override.FilenameTemplate
	}
	if template == "" {
		template = DefaultTemplate
	}
	sourceTitle := strings.TrimSpace(job.SourceTitle)
	ext := mediaExtension(job.MediaType)
	if strings.HasSuffix(strings.ToLower(sourceTitle), ext) {
		sourceTitle = sourceTitle[:len(sourceTitle)-len(ext)]
	}
	substitutions := map[string]string{
		"job_name":  cleanField(sourceTitle, "print"),
		"submitter": cleanField(job.Submitter, "unknown"),
		"timestamp": job.AcceptedAt.UTC().Format("20060102-150405"),
		"job_id":    job.JobID,
	}
	filename, err := render(template, substitutions)
	if err != nil {
		return Plan{}, err
	}
	if strings.HasSuffix(strings.ToLower(filename), strings.ToLower(mediaExtension(job.MediaType))) {
		filename = filename[:len(filename)-len(mediaExtension(job.MediaType))]
	}
	if err := validateComponent(filename); err != nil {
		return Plan{}, fmt.Errorf("export filename stem: %w", err)
	}
	filename += mediaExtension(job.MediaType)
	if len(filename) > maxFilename {
		return Plan{}, errors.New("export filename exceeds limit")
	}
	if err := validateComponent(filename); err != nil {
		return Plan{}, fmt.Errorf("export filename: %w", err)
	}
	return Plan{RelativePath: path.Join(normalizedFolder, filename), Folder: normalizedFolder, Filename: filename, MediaType: job.MediaType}, nil
}

func cleanField(raw, fallback string) string {
	var b strings.Builder
	separated := false
	for _, r := range strings.TrimSpace(raw) {
		if b.Len() >= 64 {
			break
		}
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			if separated && b.Len() > 0 {
				b.WriteByte('-')
			}
			separated = false
			b.WriteRune(r)
		} else {
			separated = true
		}
	}
	result := strings.Trim(b.String(), "-_")
	if result == "" {
		return fallback
	}
	return result
}

func render(template string, values map[string]string) (string, error) {
	if len(template) > 180 {
		return "", errors.New("export template exceeds limit")
	}
	remaining := template
	for key, value := range values {
		remaining = strings.ReplaceAll(remaining, "{"+key+"}", value)
	}
	if strings.ContainsAny(remaining, "{}") {
		return "", errors.New("unknown or malformed export template field")
	}
	if strings.ContainsAny(remaining, `/\\<>:"|?*`) {
		return "", errors.New("export template contains a path or forbidden character")
	}
	for _, c := range remaining {
		if unicode.IsControl(c) {
			return "", errors.New("export template contains control characters")
		}
	}
	remaining = strings.TrimSpace(remaining)
	if remaining == "" {
		return "", errors.New("export filename is empty")
	}
	return remaining, nil
}

func validateFolder(raw string) (string, error) {
	if raw == "" || raw == "." {
		return ".", nil
	}
	if len(raw) > maxFolder || strings.HasPrefix(raw, "/") || strings.Contains(raw, `\`) {
		return "", errors.New("export folder is not a confined relative path")
	}
	components := strings.Split(raw, "/")
	if len(components) > 6 {
		return "", errors.New("export folder exceeds depth limit")
	}
	for _, c := range components {
		if err := validateComponent(c); err != nil {
			return "", fmt.Errorf("export folder: %w", err)
		}
	}
	return strings.Join(components, "/"), nil
}

func validateComponent(c string) error {
	if c == "" || c == "." || c == ".." || strings.HasPrefix(c, ".") || len(c) > 180 || strings.HasSuffix(c, ".") || strings.HasSuffix(c, " ") {
		return errors.New("empty, hidden, dot, or invalid component")
	}
	if strings.ContainsAny(c, `/\\<>:"|?*`) {
		return errors.New("forbidden component character")
	}
	for _, ch := range c {
		if unicode.IsControl(ch) {
			return errors.New("control character")
		}
	}
	stem := strings.ToUpper(strings.SplitN(c, ".", 2)[0])
	if stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL" || len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && stem[3] >= '1' && stem[3] <= '9' {
		return errors.New("reserved Windows device name")
	}
	return nil
}

func mediaExtension(media string) string {
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
