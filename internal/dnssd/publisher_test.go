//go:build linux

package dnssd

import (
	"encoding/binary"
	"net"
	"strings"
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

func TestAirPrintTXTIsSharedAcrossDiscoveryBackends(t *testing.T) {
	identity := fixtureIdentity()
	identity.Location = "RDTE"
	got := airPrintTXT(identity)
	for _, want := range []string{
		"rp=printers/FolioRelay",
		"pdl=application/pdf,image/urf",
		"URF=V1.4,CP1,W8,PQ4,RS300,FN3",
		"UUID=01234567-89ab-4def-8123-456789abcdef",
		"note=RDTE",
	} {
		found := false
		for _, value := range got {
			if value == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("shared TXT projection missing %q: %#v", want, got)
		}
	}
}

func TestAvahiRegistrationPreservesStablePrinterIdentity(t *testing.T) {
	identity := fixtureIdentity()
	identity.Location = "RDTE"
	reg, err := makeAvahiRegistration(
		identity,
		"FolioRelay",
		&net.Interface{Index: 7, Name: "eth0"},
		net.IPv4(192, 0, 2, 10),
	)
	if err != nil {
		t.Fatal(err)
	}
	if reg.Interface != 7 || reg.Protocol != avahiProtoIPv4 {
		t.Fatalf("unexpected Avahi interface/protocol: %#v", reg)
	}
	if reg.AddressFlags != avahiPublishNoReverse ||
		reg.ServiceFlags != avahiPublishNoCookie ||
		reg.SubtypeFlags != avahiPublishFlags {
		t.Fatalf("unexpected Avahi publication flags: %#v", reg)
	}
	if reg.Name != "FolioRelay" || reg.ServiceType != "_ipp._tcp" || reg.Domain != "local" {
		t.Fatalf("unexpected Avahi service identity: %#v", reg)
	}
	if reg.Host != "foliorelay.local" || reg.Address != "192.0.2.10" || reg.Port != 8634 {
		t.Fatalf("unexpected Avahi host projection: %#v", reg)
	}
	if reg.Subtype != "_universal._sub._ipp._tcp" {
		t.Fatalf("unexpected Avahi subtype: %q", reg.Subtype)
	}
	txt := make(map[string]bool, len(reg.TXT))
	for _, item := range reg.TXT {
		txt[string(item)] = true
	}
	for _, want := range airPrintTXT(identity) {
		if !txt[want] {
			t.Fatalf("Avahi TXT missing shared projection %q", want)
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
