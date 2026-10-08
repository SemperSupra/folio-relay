package printer

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var uuidPattern = regexp.MustCompile(`^urn:uuid:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type Identity struct {
	DisplayName  string `json:"display_name"`
	Location     string `json:"location,omitempty"`
	PrinterUUID  string `json:"printer_uuid"`
	Scheme       string `json:"scheme"`
	Host         string `json:"host"`
	Port         int    `json:"port"`
	ResourcePath string `json:"resource_path"`
}

func New(displayName, location, publicURI string) (Identity, error) {
	if displayName == "" || len(displayName) > 127 {
		return Identity{}, errors.New("printer display name must contain 1-127 characters")
	}
	if len(location) > 255 {
		return Identity{}, errors.New("printer location exceeds 255 characters")
	}
	scheme, host, port, resourcePath, err := parsePublicURI(publicURI)
	if err != nil {
		return Identity{}, err
	}
	uuid, err := newUUID()
	if err != nil {
		return Identity{}, err
	}
	identity := Identity{
		DisplayName: displayName, Location: location, PrinterUUID: uuid,
		Scheme: scheme, Host: host, Port: port, ResourcePath: resourcePath,
	}
	return identity, identity.Validate()
}

func Load(path string) (Identity, error) {
	if path == "" {
		return Identity{}, errors.New("printer identity path is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Identity{}, fmt.Errorf("read printer identity: %w", err)
	}
	var identity Identity
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&identity); err != nil {
		return Identity{}, fmt.Errorf("decode printer identity: %w", err)
	}
	if err := identity.Validate(); err != nil {
		return Identity{}, fmt.Errorf("validate printer identity: %w", err)
	}
	return identity, nil
}

func LoadOrCreate(path, displayName, location, publicURI string) (Identity, error) {
	identity, err := Load(path)
	if err == nil {
		if err := ensureIdentityGroupReadable(path); err != nil {
			return Identity{}, err
		}
		return identity, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return Identity{}, err
	}

	identity, err = New(displayName, location, publicURI)
	if err != nil {
		return Identity{}, err
	}
	if err := persist(path, identity); err != nil {
		return Identity{}, err
	}
	return identity, nil
}

func (i Identity) Validate() error {
	if i.DisplayName == "" || len(i.DisplayName) > 127 || containsControl(i.DisplayName) {
		return errors.New("invalid printer display name")
	}
	if len(i.Location) > 255 || containsControl(i.Location) {
		return errors.New("invalid printer location")
	}
	if !uuidPattern.MatchString(i.PrinterUUID) {
		return errors.New("invalid printer UUID")
	}
	_, host, port, resourcePath, err := parsePublicURI(i.URI())
	if err != nil {
		return err
	}
	if host != i.Host || port != i.Port || resourcePath != i.ResourcePath {
		return errors.New("printer identity URI fields are not canonical")
	}
	return nil
}

func (i Identity) URI() string {
	if i.Scheme == "" || i.Host == "" || i.Port < 1 || i.ResourcePath == "" {
		return ""
	}
	return (&url.URL{
		Scheme: i.Scheme,
		Host: net.JoinHostPort(i.Host, strconv.Itoa(i.Port)),
		Path: i.ResourcePath,
	}).String()
}

func parsePublicURI(raw string) (string, string, int, string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", 0, "", fmt.Errorf("parse public printer URI: %w", err)
	}
	if u.Scheme != "ipp" && u.Scheme != "ipps" {
		return "", "", 0, "", errors.New("public printer URI scheme must be ipp or ipps")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", "", 0, "", errors.New("public printer URI must not contain credentials, query, or fragment")
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "" {
		return "", "", 0, "", errors.New("public printer URI host is required")
	}
	if host == "localhost" {
		return "", "", 0, "", errors.New("public printer URI cannot use localhost")
	}
	if ip := net.ParseIP(host); ip != nil && (ip.IsLoopback() || ip.IsUnspecified()) {
		return "", "", 0, "", errors.New("public printer URI cannot use a loopback or unspecified address")
	}
	port := 631
	if rawPort := u.Port(); rawPort != "" {
		parsed, err := strconv.Atoi(rawPort)
		if err != nil || parsed < 1 || parsed > 65535 {
			return "", "", 0, "", errors.New("public printer URI port is invalid")
		}
		port = parsed
	}
	if u.Path == "" || u.Path == "/" || !strings.HasPrefix(u.Path, "/") ||
		strings.ContainsAny(u.Path, " \t\r\n\x00?#%") {
		return "", "", 0, "", errors.New("public printer URI resource path is invalid")
	}
	return u.Scheme, host, port, u.Path, nil
}

func newUUID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate printer UUID: %w", err)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("urn:uuid:%x-%x-%x-%x-%x",
		value[0:4], value[4:6], value[6:8], value[8:10], value[10:16]), nil
}

func persist(path string, identity Identity) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create printer identity directory: %w", err)
	}
	if err := os.Chmod(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("chmod printer identity directory: %w", err)
	}
	data, err := json.MarshalIndent(identity, "", "  ")
	if err != nil {
		return fmt.Errorf("encode printer identity: %w", err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(path), ".printer-identity-*")
	if err != nil {
		return fmt.Errorf("create printer identity staging file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		_ = tmp.Close()
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0o640); err != nil {
		return fmt.Errorf("chmod printer identity: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write printer identity: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("fsync printer identity: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close printer identity: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("commit printer identity: %w", err)
	}
	cleanup = false
	if dir, err := os.Open(filepath.Dir(path)); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return ensureIdentityGroupReadable(path)
}

// The canonical printer identity contains only public discovery metadata. Keep
// ownership with the control-plane uid/gid, but allow the same numeric group to
// read it so the Avahi publisher can use a host-recognized unprivileged UID
// without gaining control-plane write access.
func ensureIdentityGroupReadable(path string) error {
	if err := os.Chmod(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("chmod printer identity directory: %w", err)
	}
	if err := os.Chmod(path, 0o640); err != nil {
		return fmt.Errorf("chmod printer identity: %w", err)
	}
	return nil
}

func containsControl(value string) bool {
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}
