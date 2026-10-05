//go:build linux

package dnssd

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/SemperSupra/folio-relay/internal/airprint"
	frprinter "github.com/SemperSupra/folio-relay/internal/printer"
)

const (
	mdnsPort   = 5353
	ptrType    = 12
	txtType    = 16
	srvType    = 33
	aType      = 1
	inClass    = 1
	cacheFlush = 0x8000
)

var mdnsIPv4 = net.IPv4(224, 0, 0, 251)

type Config struct {
	Identity  frprinter.Identity
	Instance  string
	Interface string
}

func Run(ctx context.Context, cfg Config) error {
	if err := cfg.Identity.Validate(); err != nil {
		return fmt.Errorf("validate printer identity: %w", err)
	}
	if !strings.HasSuffix(strings.ToLower(cfg.Identity.Host), ".local") {
		return fmt.Errorf("AirPrint DNS-SD host must be in .local: %s", cfg.Identity.Host)
	}
	instance := strings.TrimSpace(cfg.Instance)
	if instance == "" {
		instance = cfg.Identity.DisplayName
	}
	instance = dnsLabel(instance)
	if instance == "" {
		return errors.New("DNS-SD service instance is empty")
	}

	ifi, ip, err := selectIPv4Interface(cfg.Interface)
	if err != nil {
		return err
	}
	conn, err := openMDNSSocket(ifi, ip)
	if err != nil {
		return err
	}
	defer conn.Close()

	response, err := buildResponse(cfg.Identity, instance, ip, 120, 4500)
	if err != nil {
		return err
	}
	goodbye, err := buildResponse(cfg.Identity, instance, ip, 0, 0)
	if err != nil {
		return err
	}
	relevant := relevantNames(cfg.Identity, instance)
	multicast := &net.UDPAddr{IP: mdnsIPv4, Port: mdnsPort}

	if _, err := conn.WriteToUDP(response, multicast); err != nil {
		return fmt.Errorf("announce DNS-SD service: %w", err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- answerQueries(ctx, conn, multicast, response, relevant)
	}()

	second := time.NewTimer(time.Second)
	defer second.Stop()
	announcements := time.NewTicker(60 * time.Second)
	defer announcements.Stop()

	for {
		select {
		case <-ctx.Done():
			_, _ = conn.WriteToUDP(goodbye, multicast)
			time.Sleep(50 * time.Millisecond)
			_, _ = conn.WriteToUDP(goodbye, multicast)
			return nil
		case err := <-errCh:
			if err == nil || errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		case <-second.C:
			if _, err := conn.WriteToUDP(response, multicast); err != nil {
				return fmt.Errorf("repeat DNS-SD announcement: %w", err)
			}
		case <-announcements.C:
			if _, err := conn.WriteToUDP(response, multicast); err != nil {
				return fmt.Errorf("refresh DNS-SD announcement: %w", err)
			}
		}
	}
}

func answerQueries(ctx context.Context, conn *net.UDPConn, multicast *net.UDPAddr, response []byte, relevant map[string]struct{}) error {
	buf := make([]byte, 64*1024)
	for {
		if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
			return fmt.Errorf("set mDNS read deadline: %w", err)
		}
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
					continue
				}
			}
			return fmt.Errorf("read mDNS query: %w", err)
		}
		if queryMatches(buf[:n], relevant) {
			if _, err := conn.WriteToUDP(response, multicast); err != nil {
				return fmt.Errorf("answer mDNS query: %w", err)
			}
		}
	}
}

func selectIPv4Interface(requested string) (*net.Interface, net.IP, error) {
	if requested != "" {
		ifi, err := net.InterfaceByName(requested)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve requested interface %q: %w", requested, err)
		}
		ip, err := interfaceIPv4(ifi)
		if err != nil {
			return nil, nil, err
		}
		return ifi, ip, nil
	}

	probe, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: mdnsIPv4, Port: mdnsPort})
	if err == nil {
		local := probe.LocalAddr().(*net.UDPAddr).IP.To4()
		_ = probe.Close()
		if local != nil && !local.IsLoopback() {
			if ifi, err := interfaceForIP(local); err == nil {
				return ifi, local, nil
			}
		}
	}

	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, nil, fmt.Errorf("list network interfaces: %w", err)
	}
	for i := range interfaces {
		ifi := &interfaces[i]
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagLoopback != 0 || ifi.Flags&net.FlagMulticast == 0 {
			continue
		}
		if ip, err := interfaceIPv4(ifi); err == nil {
			return ifi, ip, nil
		}
	}
	return nil, nil, errors.New("no active multicast-capable IPv4 interface found")
}

func interfaceForIP(ip net.IP) (*net.Interface, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	for i := range interfaces {
		addrs, err := interfaces[i].Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var candidate net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				candidate = v.IP
			case *net.IPAddr:
				candidate = v.IP
			}
			if candidate != nil && candidate.To4() != nil && candidate.Equal(ip) {
				return &interfaces[i], nil
			}
		}
	}
	return nil, fmt.Errorf("no interface owns IPv4 address %s", ip)
}

func interfaceIPv4(ifi *net.Interface) (net.IP, error) {
	if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagLoopback != 0 || ifi.Flags&net.FlagMulticast == 0 {
		return nil, fmt.Errorf("interface %s is not active multicast-capable LAN interface", ifi.Name)
	}
	addrs, err := ifi.Addrs()
	if err != nil {
		return nil, fmt.Errorf("read addresses for %s: %w", ifi.Name, err)
	}
	for _, addr := range addrs {
		var ip net.IP
		switch v := addr.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}
		if v4 := ip.To4(); v4 != nil && !v4.IsLoopback() && !v4.IsUnspecified() {
			return v4, nil
		}
	}
	return nil, fmt.Errorf("interface %s has no usable IPv4 address", ifi.Name)
}

type socketOption struct {
	level int
	name  int
	value int
}

func mdnsSocketOptions() []socketOption {
	return []socketOption{
		{syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1},
		{syscall.SOL_SOCKET, syscall.SO_REUSEPORT, 1},
		{syscall.IPPROTO_IP, syscall.IP_MULTICAST_TTL, 255},
		{syscall.IPPROTO_IP, syscall.IP_MULTICAST_LOOP, 1},
	}
}

func openMDNSSocket(ifi *net.Interface, ip net.IP) (*net.UDPConn, error) {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, syscall.IPPROTO_UDP)
	if err != nil {
		return nil, fmt.Errorf("create mDNS socket: %w", err)
	}
	closeFD := true
	defer func() {
		if closeFD {
			_ = syscall.Close(fd)
		}
	}()
	for _, opt := range mdnsSocketOptions() {
		if err := syscall.SetsockoptInt(fd, opt.level, opt.name, opt.value); err != nil {
			return nil, fmt.Errorf("configure mDNS socket: %w", err)
		}
	}
	if err := syscall.Bind(fd, &syscall.SockaddrInet4{Port: mdnsPort}); err != nil {
		return nil, fmt.Errorf("bind UDP/%d for mDNS: %w", mdnsPort, err)
	}
	v4 := ip.To4()
	if v4 == nil {
		return nil, fmt.Errorf("interface %s did not resolve to IPv4", ifi.Name)
	}
	var local [4]byte
	copy(local[:], v4)
	if err := syscall.SetsockoptInet4Addr(fd, syscall.IPPROTO_IP, syscall.IP_MULTICAST_IF, local); err != nil {
		return nil, fmt.Errorf("select mDNS multicast interface %s: %w", ifi.Name, err)
	}
	mreq := &syscall.IPMreq{Multiaddr: [4]byte{224, 0, 0, 251}, Interface: local}
	if err := syscall.SetsockoptIPMreq(fd, syscall.IPPROTO_IP, syscall.IP_ADD_MEMBERSHIP, mreq); err != nil {
		return nil, fmt.Errorf("join mDNS multicast group on %s: %w", ifi.Name, err)
	}
	file := os.NewFile(uintptr(fd), "foliorelay-mdns")
	if file == nil {
		return nil, errors.New("wrap mDNS socket file descriptor")
	}
	pc, err := net.FilePacketConn(file)
	_ = file.Close()
	if err != nil {
		return nil, fmt.Errorf("adopt mDNS socket: %w", err)
	}
	closeFD = false
	udp, ok := pc.(*net.UDPConn)
	if !ok {
		_ = pc.Close()
		return nil, errors.New("mDNS socket is not UDP")
	}
	return udp, nil
}

func relevantNames(identity frprinter.Identity, instance string) map[string]struct{} {
	service := "_ipp._tcp.local."
	subtype := "_universal._sub._ipp._tcp.local."
	instanceName := strings.ToLower(instance + "." + service)
	host := strings.ToLower(strings.TrimSuffix(identity.Host, ".") + ".")
	return map[string]struct{}{
		service:                          {},
		subtype:                          {},
		instanceName:                     {},
		host:                             {},
		"_services._dns-sd._udp.local.": {},
	}
}

func queryMatches(msg []byte, relevant map[string]struct{}) bool {
	if len(msg) < 12 || binary.BigEndian.Uint16(msg[2:4])&0x8000 != 0 {
		return false
	}
	questions := int(binary.BigEndian.Uint16(msg[4:6]))
	off := 12
	for i := 0; i < questions; i++ {
		name, next, err := readName(msg, off)
		if err != nil || next+4 > len(msg) {
			return false
		}
		off = next + 4
		if _, ok := relevant[strings.ToLower(name)]; ok {
			return true
		}
	}
	return false
}

func readName(msg []byte, off int) (string, int, error) {
	if off < 0 || off >= len(msg) {
		return "", 0, errors.New("DNS name offset out of range")
	}
	labels := make([]string, 0, 8)
	pos := off
	next := -1
	jumps := 0
	for {
		if pos >= len(msg) {
			return "", 0, errors.New("truncated DNS name")
		}
		length := int(msg[pos])
		if length&0xc0 == 0xc0 {
			if pos+1 >= len(msg) {
				return "", 0, errors.New("truncated DNS compression pointer")
			}
			ptr := ((length & 0x3f) << 8) | int(msg[pos+1])
			if ptr >= len(msg) || jumps > 16 {
				return "", 0, errors.New("invalid DNS compression pointer")
			}
			if next < 0 {
				next = pos + 2
			}
			pos = ptr
			jumps++
			continue
		}
		if length&0xc0 != 0 {
			return "", 0, errors.New("invalid DNS label length")
		}
		pos++
		if length == 0 {
			if next < 0 {
				next = pos
			}
			break
		}
		if length > 63 || pos+length > len(msg) {
			return "", 0, errors.New("invalid DNS label")
		}
		labels = append(labels, string(msg[pos:pos+length]))
		pos += length
	}
	return strings.Join(labels, ".") + ".", next, nil
}

func buildResponse(identity frprinter.Identity, instance string, ip net.IP, uniqueTTL, sharedTTL uint32) ([]byte, error) {
	service := []string{"_ipp", "_tcp", "local"}
	subtype := []string{"_universal", "_sub", "_ipp", "_tcp", "local"}
	meta := []string{"_services", "_dns-sd", "_udp", "local"}
	instanceName := append([]string{instance}, service...)
	host := strings.Split(strings.TrimSuffix(identity.Host, "."), ".")
	if err := validateLabels(host); err != nil {
		return nil, fmt.Errorf("invalid identity host for DNS-SD: %w", err)
	}

	rp := strings.TrimPrefix(identity.ResourcePath, "/")
	uuid := strings.TrimPrefix(identity.PrinterUUID, "urn:uuid:")
	txt := []string{
		"rp=" + rp,
		"pdl=" + airprint.PDL,
		"URF=" + airprint.URF,
		"product=" + airprint.Product,
		"ty=" + airprint.PrinterType,
		"txtvers=1",
		"qtotal=1",
		"UUID=" + uuid,
	}
	if identity.Location != "" {
		txt = append(txt, "note="+identity.Location)
	}
	txtData, err := encodeTXT(txt)
	if err != nil {
		return nil, err
	}
	ptrInstance, err := encodeName(instanceName)
	if err != nil {
		return nil, err
	}
	ptrService, err := encodeName(service)
	if err != nil {
		return nil, err
	}
	hostData, err := encodeName(host)
	if err != nil {
		return nil, err
	}
	srvData := make([]byte, 6)
	binary.BigEndian.PutUint16(srvData[4:6], uint16(identity.Port))
	srvData = append(srvData, hostData...)
	v4 := ip.To4()
	if v4 == nil {
		return nil, errors.New("DNS-SD publisher requires IPv4")
	}

	records := make([]byte, 0, 1024)
	records = append(records, resourceRecord(meta, ptrType, inClass, sharedTTL, ptrService)...)
	records = append(records, resourceRecord(service, ptrType, inClass, sharedTTL, ptrInstance)...)
	records = append(records, resourceRecord(subtype, ptrType, inClass, sharedTTL, ptrInstance)...)
	records = append(records, resourceRecord(instanceName, srvType, inClass|cacheFlush, uniqueTTL, srvData)...)
	records = append(records, resourceRecord(instanceName, txtType, inClass|cacheFlush, uniqueTTL, txtData)...)
	records = append(records, resourceRecord(host, aType, inClass|cacheFlush, uniqueTTL, v4)...)

	msg := make([]byte, 12, 12+len(records))
	binary.BigEndian.PutUint16(msg[2:4], 0x8400)
	binary.BigEndian.PutUint16(msg[6:8], 6)
	msg = append(msg, records...)
	return msg, nil
}

func resourceRecord(name []string, typ, class uint16, ttl uint32, rdata []byte) []byte {
	encoded, err := encodeName(name)
	if err != nil {
		panic(err)
	}
	buf := make([]byte, 0, len(encoded)+10+len(rdata))
	buf = append(buf, encoded...)
	header := make([]byte, 10)
	binary.BigEndian.PutUint16(header[0:2], typ)
	binary.BigEndian.PutUint16(header[2:4], class)
	binary.BigEndian.PutUint32(header[4:8], ttl)
	binary.BigEndian.PutUint16(header[8:10], uint16(len(rdata)))
	buf = append(buf, header...)
	buf = append(buf, rdata...)
	return buf
}

func encodeName(labels []string) ([]byte, error) {
	if err := validateLabels(labels); err != nil {
		return nil, err
	}
	out := make([]byte, 0, 64)
	for _, label := range labels {
		out = append(out, byte(len([]byte(label))))
		out = append(out, []byte(label)...)
	}
	out = append(out, 0)
	return out, nil
}

func validateLabels(labels []string) error {
	if len(labels) == 0 {
		return errors.New("DNS name has no labels")
	}
	total := 1
	for _, label := range labels {
		length := len([]byte(label))
		if length == 0 || length > 63 {
			return fmt.Errorf("DNS label length %d is invalid", length)
		}
		total += 1 + length
	}
	if total > 255 {
		return fmt.Errorf("DNS name length %d exceeds 255 bytes", total)
	}
	return nil
}

func encodeTXT(values []string) ([]byte, error) {
	out := make([]byte, 0, 256)
	for _, value := range values {
		if len([]byte(value)) > 255 {
			return nil, fmt.Errorf("DNS-SD TXT item exceeds 255 bytes: %q", value)
		}
		out = append(out, byte(len([]byte(value))))
		out = append(out, []byte(value)...)
	}
	return out, nil
}

func dnsLabel(value string) string {
	value = strings.TrimSpace(value)
	if len([]byte(value)) <= 63 {
		return value
	}
	for len([]byte(value)) > 63 {
		_, size := utf8.DecodeLastRuneInString(value)
		if size <= 0 {
			return ""
		}
		value = value[:len(value)-size]
	}
	return strings.TrimSpace(value)
}
