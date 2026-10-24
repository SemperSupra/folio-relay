package inbox

import (
	"fmt"
	"strings"
	"unicode"
)

// CleanTitle keeps optional, untrusted print-job text out of control headers
// while retaining a useful label in the Inbox. It does not affect job identity.
func CleanTitle(raw string) string {
	var out []rune
	for _, r := range strings.TrimSpace(raw) {
		if unicode.IsControl(r) {
			continue
		}
		out = append(out, r)
		if len(out) == 160 {
			break
		}
	}
	return strings.TrimSpace(string(out))
}

func mediaExtension(mediaType string) string {
	switch strings.ToLower(strings.TrimSpace(strings.SplitN(mediaType, ";", 2)[0])) {
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

// filenamePart permits only ASCII filename characters, so the result is safe
// in a quoted Content-Disposition filename and cannot name a path.
func filenamePart(raw string) string {
	var b strings.Builder
	separator := false
	for _, r := range raw {
		if b.Len() >= 64 {
			break
		}
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			if separator && b.Len() > 0 {
				b.WriteByte('-')
			}
			separator = false
			b.WriteRune(r)
		} else if r == '-' || r == '_' {
			if separator && b.Len() > 0 {
				b.WriteByte('-')
			}
			separator = false
			b.WriteRune(r)
		} else {
			separator = true
		}
	}
	return strings.Trim(b.String(), "-_")
}

// DownloadName uses the original CUPS title when available. Older records
// remain downloadable with source, accepted time, and a stable unique suffix.
func DownloadName(record Record) string {
	ext := mediaExtension(record.MediaType)
	title := CleanTitle(record.SourceTitle)
	if strings.HasSuffix(strings.ToLower(title), ext) {
		title = title[:len(title)-len(ext)]
	}
	base := filenamePart(title)
	if base == "" {
		base = filenamePart(record.Substrate + "-" + record.SubstrateJobID)
	}
	if base == "" {
		base = "print"
	}
	id := strings.TrimPrefix(record.JobID, "job-")
	if len(id) > 12 {
		id = id[:12]
	}
	if id == "" {
		id = "unknown"
	}
	return fmt.Sprintf("%s-%s-%s%s", base, record.AcceptedAt.UTC().Format("20060102-150405"), id, ext)
}
