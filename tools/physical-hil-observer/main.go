package main

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

const (
	queryName     = "_universal._sub._ipp._tcp.local"
	legacyQueryID = uint16(0x4652)
)

type srvRecord struct {
	target string
	port   int
}

type observation struct {
	universalTargets []string
	txtByOwner       map[string][]string
	srvByOwner       map[string]srvRecord
}

type serviceMatch struct {
	instance  string
	txt       []string
	srvTarget string
	srvPort   int
}

func newObservation() observation {
	return observation{
		txtByOwner: map[string][]string{},
		srvByOwner: map[string]srvRecord{},
	}
}

func (o *observation) merge(other observation) {
	o.universalTargets = append(o.universalTargets, other.universalTargets...)
	for owner, items := range other.txtByOwner {
		o.txtByOwner[owner] = append(o.txtByOwner[owner], items...)
	}
	for owner, srv := range other.srvByOwner {
		o.srvByOwner[owner] = srv
	}
}

func encodeName(name string) []byte {
	out := make([]byte, 0, len(name)+2)
	for _, label := range strings.Split(name, ".") {
		out = append(out, byte(len(label)))
		out = append(out, label...)
	}
	return append(out, 0)
}

func nameAt(pkt []byte, off int, seen map[int]bool) (string, int, error) {
	if seen == nil {
		seen = map[int]bool{}
	}
	labels := []string{}
	end := -1
	for {
		if off >= len(pkt) {
			return "", 0, errors.New("dns name overflow")
		}
		n := int(pkt[off])
		if n == 0 {
			if end < 0 {
				end = off + 1
			}
			return strings.Join(labels, "."), end, nil
		}
		if n&0xC0 == 0xC0 {
			if off+1 >= len(pkt) {
				return "", 0, errors.New("dns pointer overflow")
			}
			ptr := ((n & 0x3f) << 8) | int(pkt[off+1])
			if seen[ptr] {
				return "", 0, errors.New("dns pointer loop")
			}
			seen[ptr] = true
			tail, _, err := nameAt(pkt, ptr, seen)
			if err != nil {
				return "", 0, err
			}
			if tail != "" {
				labels = append(labels, tail)
			}
			if end < 0 {
				end = off + 2
			}
			return strings.Join(labels, "."), end, nil
		}
		off++
		if n > 63 || off+n > len(pkt) {
			return "", 0, errors.New("dns label overflow")
		}
		labels = append(labels, string(pkt[off:off+n]))
		off += n
	}
}

func normalizeDNSName(name string) string {
	return strings.ToLower(strings.TrimSuffix(name, "."))
}

func parsePacket(pkt []byte) (observation, error) {
	o := newObservation()
	if len(pkt) < 12 {
		return o, errors.New("short dns packet")
	}
	qd := int(binary.BigEndian.Uint16(pkt[4:6]))
	an := int(binary.BigEndian.Uint16(pkt[6:8]))
	ns := int(binary.BigEndian.Uint16(pkt[8:10]))
	ar := int(binary.BigEndian.Uint16(pkt[10:12]))
	off := 12
	for i := 0; i < qd; i++ {
		_, next, err := nameAt(pkt, off, nil)
		if err != nil {
			return o, err
		}
		off = next
		if off+4 > len(pkt) {
			return o, errors.New("short dns question")
		}
		off += 4
	}
	for i := 0; i < an+ns+ar; i++ {
		name, next, err := nameAt(pkt, off, nil)
		if err != nil {
			return o, err
		}
		off = next
		if off+10 > len(pkt) {
			return o, errors.New("short dns rr")
		}
		typ := binary.BigEndian.Uint16(pkt[off : off+2])
		rdlen := int(binary.BigEndian.Uint16(pkt[off+8 : off+10]))
		off += 10
		rstart := off
		if off+rdlen > len(pkt) {
			return o, errors.New("short dns rdata")
		}
		rdata := pkt[off : off+rdlen]
		off += rdlen
		owner := normalizeDNSName(name)

		if owner == normalizeDNSName(queryName) && typ == 12 {
			target, _, err := nameAt(pkt, rstart, nil)
			if err != nil {
				return o, err
			}
			o.universalTargets = append(o.universalTargets, target)
		}
		if typ == 16 {
			items := []string{}
			for j := 0; j < len(rdata); {
				n := int(rdata[j])
				j++
				if j+n > len(rdata) {
					return o, errors.New("short dns txt item")
				}
				items = append(items, string(rdata[j:j+n]))
				j += n
			}
			o.txtByOwner[owner] = append(o.txtByOwner[owner], items...)
		}
		if typ == 33 && rdlen >= 7 {
			target, _, err := nameAt(pkt, rstart+6, nil)
			if err != nil {
				return o, err
			}
			o.srvByOwner[owner] = srvRecord{
				target: target,
				port:   int(binary.BigEndian.Uint16(rdata[4:6])),
			}
		}
	}
	return o, nil
}

func matchObservation(o observation, expectedTxtUUID, expectedHost, expectedRP string, expectedPort int) (serviceMatch, bool) {
	for _, instance := range o.universalTargets {
		owner := normalizeDNSName(instance)
		txt, ok := o.txtByOwner[owner]
		if !ok {
			continue
		}
		srv, ok := o.srvByOwner[owner]
		if !ok {
			continue
		}
		uuidMatch := false
		rpMatch := false
		pdlMatch := false
		for _, item := range txt {
			if strings.EqualFold(item, "UUID="+expectedTxtUUID) {
				uuidMatch = true
			}
			if strings.EqualFold(item, "rp="+expectedRP) {
				rpMatch = true
			}
			if strings.EqualFold(item, "pdl=application/pdf,image/urf") {
				pdlMatch = true
			}
		}
		if uuidMatch && rpMatch && pdlMatch &&
			strings.EqualFold(normalizeDNSName(srv.target), normalizeDNSName(expectedHost)) &&
			srv.port == expectedPort {
			return serviceMatch{
				instance:  instance,
				txt:       txt,
				srvTarget: srv.target,
				srvPort:   srv.port,
			}, true
		}
	}
	return serviceMatch{}, false
}

func buildQuery() []byte {
	q := make([]byte, 12)
	binary.BigEndian.PutUint16(q[0:2], legacyQueryID)
	binary.BigEndian.PutUint16(q[4:6], 1)
	q = append(q, encodeName(queryName)...)
	q = append(q, 0, 12, 0, 1)
	return q
}

func observe(expectedUUID, expectedHost, expectedResourcePath string, expectedPort int, seconds float64) (map[string]any, error) {
	txtUUID := strings.TrimPrefix(strings.ToLower(expectedUUID), "urn:uuid:")
	if txtUUID == expectedUUID || txtUUID == "" {
		return nil, errors.New("expected UUID must use canonical urn:uuid form")
	}
	expectedRP := strings.TrimPrefix(expectedResourcePath, "/")
	if expectedRP == "" {
		return nil, errors.New("resource path is required")
	}

	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	dst := &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: 5353}
	query := buildQuery()
	deadline := time.Now().Add(time.Duration(seconds * float64(time.Second)))
	aggregate := newObservation()
	buf := make([]byte, 65535)

	for time.Now().Before(deadline) {
		if _, err := conn.WriteToUDP(query, dst); err != nil {
			return nil, err
		}
		window := time.Now().Add(time.Second)
		if window.After(deadline) {
			window = deadline
		}
		if err := conn.SetReadDeadline(window); err != nil {
			return nil, err
		}
		for {
			n, _, err := conn.ReadFromUDP(buf)
			if err != nil {
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					break
				}
				return nil, err
			}
			if n < 2 || binary.BigEndian.Uint16(buf[:2]) != legacyQueryID {
				continue
			}
			packetObservation, err := parsePacket(buf[:n])
			if err != nil {
				continue
			}
			aggregate.merge(packetObservation)
			if matched, ok := matchObservation(aggregate, txtUUID, expectedHost, expectedRP, expectedPort); ok {
				return map[string]any{
					"status":           "success",
					"query_transport":  "legacy-unicast",
					"universal_ptr":    true,
					"service_instance": matched.instance,
					"uuid":             expectedUUID,
					"txt":              matched.txt,
					"srv_target":       matched.srvTarget,
					"srv_port":         matched.srvPort,
					"resource_path":    expectedResourcePath,
				}, nil
			}
		}
	}
	return nil, errors.New("no qualifying _universal FolioRelay mDNS response")
}

func main() {
	uuid := flag.String("uuid", "", "expected canonical printer UUID")
	host := flag.String("expected-host", "", "expected DNS-SD SRV host")
	ippPort := flag.Int("expected-ipp-port", 0, "expected DNS-SD SRV port")
	resourcePath := flag.String("resource-path", "/printers/FolioRelay", "expected IPP resource path")
	seconds := flag.Float64("seconds", 25, "mDNS observation window")
	flag.Parse()

	if *uuid == "" || *host == "" || *ippPort <= 0 {
		fmt.Fprintln(os.Stderr, "uuid, expected-host, and expected-ipp-port are required")
		os.Exit(2)
	}
	result, err := observe(*uuid, *host, *resourcePath, *ippPort, *seconds)
	if err != nil {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
			"status": "error",
			"error":  err.Error(),
		})
		os.Exit(1)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(result)
}
