package main

import (
	"encoding/binary"
	"testing"
)

const testInstance = "FolioRelay._ipp._tcp.local"

func validObservation() observation {
	o := newObservation()
	o.baseTargets = []string{testInstance}
	o.universalTargets = []string{testInstance}
	owner := normalizeDNSName(testInstance)
	o.txtByOwner[owner] = []string{
		"UUID=01234567-89ab-4def-8123-456789abcdef",
		"rp=printers/FolioRelay",
		"pdl=application/pdf,image/urf",
	}
	o.srvByOwner[owner] = srvRecord{target: "foliorelay.local.", port: 8634}
	return o
}

func appendRR(pkt *[]byte, owner string, typ uint16, rdata []byte) {
	*pkt = append(*pkt, encodeName(owner)...)
	hdr := make([]byte, 10)
	binary.BigEndian.PutUint16(hdr[0:2], typ)
	binary.BigEndian.PutUint16(hdr[2:4], 1)
	binary.BigEndian.PutUint32(hdr[4:8], 120)
	binary.BigEndian.PutUint16(hdr[8:10], uint16(len(rdata)))
	*pkt = append(*pkt, hdr...)
	*pkt = append(*pkt, rdata...)
}

func TestBuildQueryUsesLegacyCorrelationID(t *testing.T) {
	q := buildQuery(universalQueryName)
	if got := binary.BigEndian.Uint16(q[0:2]); got != legacyQueryID {
		t.Fatalf("query id=%#x want=%#x", got, legacyQueryID)
	}
	if got := binary.BigEndian.Uint16(q[4:6]); got != 1 {
		t.Fatalf("question count=%d want=1", got)
	}
}

func TestParseAndMatchSameInstance(t *testing.T) {
	pkt := make([]byte, 12)
	binary.BigEndian.PutUint16(pkt[6:8], 4)
	appendRR(&pkt, baseQueryName, 12, encodeName(testInstance))
	appendRR(&pkt, universalQueryName, 12, encodeName(testInstance))
	txt := []byte{}
	for _, s := range []string{
		"UUID=01234567-89ab-4def-8123-456789abcdef",
		"rp=printers/FolioRelay",
		"pdl=application/pdf,image/urf",
	} {
		txt = append(txt, byte(len(s)))
		txt = append(txt, []byte(s)...)
	}
	appendRR(&pkt, testInstance, 16, txt)
	srv := make([]byte, 6)
	binary.BigEndian.PutUint16(srv[4:6], 8634)
	srv = append(srv, encodeName("foliorelay.local")...)
	appendRR(&pkt, testInstance, 33, srv)

	o, err := parsePacket(pkt)
	if err != nil {
		t.Fatal(err)
	}
	m, ok := matchObservation(o,
		"01234567-89ab-4def-8123-456789abcdef",
		"foliorelay.local",
		"printers/FolioRelay",
		8634)
	if !ok {
		t.Fatalf("expected same-instance match: %+v", o)
	}
	if m.instance != testInstance || m.srvPort != 8634 {
		t.Fatalf("unexpected match: %+v", m)
	}
}

func TestRejectsCrossInstanceSplicing(t *testing.T) {
	o := newObservation()
	good := normalizeDNSName("Good._ipp._tcp.local")
	other := normalizeDNSName("Other._ipp._tcp.local")
	o.baseTargets = []string{"Good._ipp._tcp.local"}
	o.universalTargets = []string{"Good._ipp._tcp.local"}
	o.txtByOwner[good] = []string{
		"UUID=01234567-89ab-4def-8123-456789abcdef",
		"rp=printers/FolioRelay",
		"pdl=application/pdf,image/urf",
	}
	o.srvByOwner[other] = srvRecord{target: "foliorelay.local.", port: 8634}
	if _, ok := matchObservation(o,
		"01234567-89ab-4def-8123-456789abcdef",
		"foliorelay.local",
		"printers/FolioRelay",
		8634); ok {
		t.Fatal("must not splice TXT and SRV from different service instances")
	}
}


func TestRejectsSubtypeWithoutBaseIPPPtr(t *testing.T) {
	o := validObservation()
	o.baseTargets = nil
	if _, ok := matchObservation(o,
		"01234567-89ab-4def-8123-456789abcdef",
		"foliorelay.local",
		"printers/FolioRelay",
		8634); ok {
		t.Fatal("AirPrint subtype must correlate with a base _ipp._tcp PTR")
	}
}

func TestRejectsWrongResourcePath(t *testing.T) {
	if _, ok := matchObservation(validObservation(),
		"01234567-89ab-4def-8123-456789abcdef",
		"foliorelay.local",
		"printers/Other",
		8634); ok {
		t.Fatal("unexpected match with wrong resource path")
	}
}
