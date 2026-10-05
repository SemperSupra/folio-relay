//go:build linux

package dnssd

import (
	"encoding/binary"
	"net"
	"strings"
	"syscall"
	"testing"

	frprinter "github.com/SemperSupra/folio-relay/internal/printer"
)

func fixtureIdentity() frprinter.Identity {
	return frprinter.Identity{
		DisplayName:  "FolioRelay",
		PrinterUUID:  "urn:uuid:01234567-89ab-4def-8123-456789abcdef",
		Scheme:       "ipp",
		Host:         "foliorelay.local",
		Port:         8634,
		ResourcePath: "/printers/FolioRelay",
	}
}

func TestMDNSSocketOptionsPermitSharedAvahiPort(t *testing.T) {
	want := map[[2]int]bool{
		{syscall.SOL_SOCKET, syscall.SO_REUSEADDR}: false,
		{syscall.SOL_SOCKET, soReusePort}:             false,
	}
	for _, opt := range mdnsSocketOptions() {
		key := [2]int{opt.level, opt.name}
		if _, ok := want[key]; ok && opt.value == 1 {
			want[key] = true
		}
	}
	for key, found := range want {
		if !found {
			t.Fatalf("required shared-port socket option missing: level=%d name=%d", key[0], key[1])
		}
	}
}

func TestBuildResponseContainsAirPrintProjection(t *testing.T) {
	msg, err := buildResponse(fixtureIdentity(), "FolioRelay", net.IPv4(192, 0, 2, 10), 120, 4500)
	if err != nil {
		t.Fatal(err)
	}
	if got := binary.BigEndian.Uint16(msg[6:8]); got != 6 {
		t.Fatalf("answer count=%d want 6", got)
	}
	for _, needle := range []string{
		"_universal", "_ipp", "_tcp", "FolioRelay",
		"rp=printers/FolioRelay", "pdl=application/pdf,image/urf",
		"URF=V1.4,CP1,W8,PQ4,RS300,FN3",
		"UUID=01234567-89ab-4def-8123-456789abcdef",
	} {
		if !strings.Contains(string(msg), needle) {
			t.Fatalf("response missing %q", needle)
		}
	}
}

func TestQueryMatchesSubtypeAndIgnoresResponses(t *testing.T) {
	qname, err := encodeName([]string{"_universal", "_sub", "_ipp", "_tcp", "local"})
	if err != nil {
		t.Fatal(err)
	}
	msg := make([]byte, 12)
	binary.BigEndian.PutUint16(msg[4:6], 1)
	msg = append(msg, qname...)
	msg = append(msg, 0, 12, 0, 1)
	if !queryMatches(msg, relevantNames(fixtureIdentity(), "FolioRelay")) {
		t.Fatal("expected subtype browse query to match")
	}
	binary.BigEndian.PutUint16(msg[2:4], 0x8400)
	if queryMatches(msg, relevantNames(fixtureIdentity(), "FolioRelay")) {
		t.Fatal("response packet must not trigger a reply")
	}
}

func TestDNSLabelTruncatesOnRuneBoundary(t *testing.T) {
	value := strings.Repeat("é", 40)
	got := dnsLabel(value)
	if len([]byte(got)) > 63 {
		t.Fatalf("label length=%d", len([]byte(got)))
	}
	if !strings.HasSuffix(got, "é") {
		t.Fatalf("label ended on invalid boundary: %q", got)
	}
}
