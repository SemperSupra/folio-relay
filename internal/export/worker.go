package export

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

var (
	ErrCollision = errors.New("export destination already exists")
	ErrSource = errors.New("canonical artifact invalid or unavailable")
)

// Artifact references an immutable content-addressed canonical artifact.
// Neither Publish nor Reconcile ever removes or modifies this source.
type Artifact struct {
	SHA256 string
	Bytes  int64
}

// Receipt is inert provenance for a subsequent durable export effect.
type Receipt struct {
	RelativePath string `json:"relative_path"`
	SHA256 string `json:"sha256"`
	Bytes int64 `json:"bytes"`
}

// Worker is a filesystem-confined, source-only component. Its caller must
// authorize the job and record/reconcile its effects durably; it is not an API.
type Worker struct {
	canonical *os.Root
	exports *os.Root
}

func NewWorker(canonicalDir, exportDir string) (*Worker, error) {
	canon, err := checkedRoot(canonicalDir)
	if err != nil { return nil, fmt.Errorf("canonical root: %w", err) }
	out, err := checkedRoot(exportDir)
	if err != nil { return nil, fmt.Errorf("export root: %w", err) }
	if overlaps(canon, out) {
		return nil, errors.New("canonical and export roots must not overlap")
	}
	source, err := os.OpenRoot(canon)
	if err != nil { return nil, err }
	target, err := os.OpenRoot(out)
	if err != nil { source.Close(); return nil, err }
	return &Worker{canonical: source, exports: target}, nil
}

func checkedRoot(dir string) (string, error) {
	if dir == "" { return "", errors.New("missing root") }
	st, err := os.Lstat(dir)
	if err != nil { return "", err }
	if !st.IsDir() || st.Mode() & os.ModeSymlink != 0 {
		return "", errors.New("root must be an existing real directory")
	}
	abs, err := filepath.Abs(dir)
	if err != nil { return "", err }
	real, err := filepath.EvalSymlinks(abs)
	if err != nil { return "", err }
	if real != abs {
		// Fail closed on aliases; the site operator should bind a canonical path.
		return "", errors.New("root resolves through symlinks")
	}
	return real, nil
}

func overlaps(a, b string) bool {
	rel, err := filepath.Rel(a, b)
	if err == nil && (rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) { return true }
	rel, err = filepath.Rel(b, a)
	return err == nil && (rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func (w *Worker) Close() error {
	if w == nil { return nil }
	var errs []error
	if w.canonical != nil { errs = append(errs, w.canonical.Close()) }
	if w.exports != nil { errs = append(errs, w.exports.Close()) }
	return errors.Join(errs...)
}

func validatePlan(p Plan) error {
	folder, err := validateFolder(p.Folder)
	if err != nil { return err }
	if err := validateComponent(p.Filename); err != nil { return err }
	if p.RelativePath != path.Join(folder, p.Filename) {
		return errors.New("export plan path is not canonical")
	}
	if !strings.HasSuffix(strings.ToLower(p.Filename), mediaExtension(p.MediaType)) {
		return errors.New("export plan MIME extension mismatch")
	}
	return nil
}

func validateArtifact(a Artifact) error {
	if len(a.SHA256) != 64 || a.Bytes < 0 || a.Bytes > 1<<30 {
		return ErrSource
	}
	for _, c := range a.SHA256 {
		if c < '0' || c > '9' && c < 'a' || c > 'f' {
			return ErrSource
		}
	}
	return nil
}

func (w *Worker) sourceFile(a Artifact) (*os.File, error) {
	if err := validateArtifact(a); err != nil { return nil, err }
	name := path.Join("sha256", a.SHA256[:2], a.SHA256)
	stat, err := w.canonical.Lstat(name)
	if err != nil || !stat.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: not a canonical regular file", ErrSource)
	}
	f, err := w.canonical.Open(name)
	if err != nil { return nil, fmt.Errorf("%w: open: %v", ErrSource, err) }
	stat, err = f.Stat()
	if err != nil || !stat.Mode().IsRegular() {
		f.Close()
		return nil, ErrSource
	}
	return f, nil
}

func (w *Worker) ensureFolder(folder string) error {
	if folder == "." { return nil }
	parts := strings.Split(folder, "/")
	for i := range parts {
		name := strings.Join(parts[:i+1], "/")
		err := w.exports.Mkdir(name, 0o750)
		if err != nil && !errors.Is(err, os.ErrExist) { return err }
		stat, err := w.exports.Lstat(name)
		if err != nil || !stat.IsDir() || stat.Mode() & os.ModeSymlink != 0 {
			return errors.New("export path contains an unsafe directory")
		}
	}
	return nil
}

// Publish verifies source bytes before exposing a fully written destination.
// A hardlink of an exclusive staging file provides no-clobber atomic visibility.
// Any error is fail-closed; a concurrent SMB delete may happen after success.
func (w *Worker) Publish(p Plan, a Artifact) (Receipt, error) {
	if w == nil || w.canonical == nil || w.exports == nil { return Receipt{}, errors.New("worker closed") }
	if err := validatePlan(p); err != nil { return Receipt{}, err }
	source, err := w.sourceFile(a)
	if err != nil { return Receipt{}, err }
	defer source.Close()
	if err := w.ensureFolder(p.Folder); err != nil { return Receipt{}, err }
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil { return Receipt{}, err }
	temp := path.Join(p.Folder, ".foliorelay-stage-"+hex.EncodeToString(nonce[:]))
	output, err := w.exports.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil { return Receipt{}, err }
	defer w.exports.Remove(temp)
	hash := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(output, hash), io.LimitReader(source, a.Bytes+1))
	if copyErr == nil { copyErr = output.Sync() }
	if closeErr := output.Close(); copyErr == nil { copyErr = closeErr }
	if copyErr != nil { return Receipt{}, copyErr }
	if n != a.Bytes || hex.EncodeToString(hash.Sum(nil)) != a.SHA256 {
		return Receipt{}, fmt.Errorf("%w: source size/digest mismatch", ErrSource)
	}
	// Root.Link fails if the destination exists, even if an SMB client has
	// replaced it with a link. Never delete/rewrite an existing export.
	if err := w.exports.Link(temp, p.RelativePath); err != nil {
		if errors.Is(err, os.ErrExist) { return Receipt{}, ErrCollision }
		return Receipt{}, err
	}
	return Receipt{RelativePath:p.RelativePath, SHA256:a.SHA256, Bytes:n}, nil
}

// Inspect distinguishes a matching export from an absent, replaced or altered
// export. A missing path may have been renamed or deleted by SMB clients;
// without a separate tracked rename, its correct state is "missing_or_moved".
func (w *Worker) Inspect(receipt Receipt) (string, error) {
	if w == nil || w.exports == nil { return "", errors.New("worker closed") }
	if err := validateArtifact(Artifact{SHA256:receipt.SHA256, Bytes:receipt.Bytes}); err != nil { return "", err }
	if receipt.RelativePath == "." || strings.HasPrefix(receipt.RelativePath, "/") {
		return "", errors.New("invalid export receipt")
	}
	folder, filename := path.Split(receipt.RelativePath)
	folder = strings.TrimSuffix(folder, "/")
	if folder == "" { folder = "." }
	if err := validatePlan(Plan{RelativePath:receipt.RelativePath, Folder:folder, Filename:filename, MediaType:extensionMedia(filename)}); err != nil { return "", err }
	st, err := w.exports.Lstat(receipt.RelativePath)
	if errors.Is(err, os.ErrNotExist) { return "missing_or_moved", nil }
	if err != nil { return "", err }
	if !st.Mode().IsRegular() { return "modified", nil }
	f, err := w.exports.Open(receipt.RelativePath)
	if errors.Is(err, os.ErrNotExist) { return "missing_or_moved", nil }
	if err != nil { return "", err }
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, receipt.Bytes+1))
	if err != nil { return "", err }
	if n != receipt.Bytes || hex.EncodeToString(h.Sum(nil)) != receipt.SHA256 { return "modified", nil }
	return "present", nil
}

func extensionMedia(filename string) string {
	switch strings.ToLower(path.Ext(filename)) {
	case ".pdf": return "application/pdf"
	case ".urf": return "image/urf"
	case ".pwg": return "image/pwg-raster"
	case ".ps": return "application/postscript"
	case ".jpg": return "image/jpeg"
	case ".png": return "image/png"
	case ".txt": return "text/plain"
	default: return "application/octet-stream"
	}
}
