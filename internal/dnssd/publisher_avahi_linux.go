//go:build linux

package dnssd

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/godbus/dbus/v5"

	frprinter "github.com/SemperSupra/folio-relay/internal/printer"
)

const (
	defaultAvahiDBusAddress = "unix:path=/run/dbus/system_bus_socket"
	avahiService            = "org.freedesktop.Avahi"
	avahiServerPath         = dbus.ObjectPath("/")
	avahiServerInterface    = "org.freedesktop.Avahi.Server"
	avahiEntryInterface     = "org.freedesktop.Avahi.EntryGroup"

	avahiProtoIPv4             = int32(0)
	avahiEntryUncommitted      = int32(0)
	avahiEntryRegistering      = int32(1)
	avahiEntryEstablished      = int32(2)
	avahiEntryCollision        = int32(3)
	avahiEntryFailure          = int32(4)
	avahiPublishFlags          = uint32(0)
	avahiRegistrationWait      = 10 * time.Second
	avahiRegistrationPoll      = 100 * time.Millisecond
	avahiRegistrationHealthPoll = 10 * time.Second
)

type avahiRegistration struct {
	Interface   int32
	Protocol    int32
	Flags       uint32
	Name        string
	ServiceType string
	Domain      string
	Host        string
	Address     string
	Port        uint16
	Subtype     string
	TXT         [][]byte
}

func makeAvahiRegistration(identity frprinter.Identity, instance string, ifi *net.Interface, ip net.IP) (avahiRegistration, error) {
	if ifi == nil || ifi.Index <= 0 {
		return avahiRegistration{}, fmt.Errorf("Avahi publication requires a concrete network interface")
	}
	v4 := ip.To4()
	if v4 == nil {
		return avahiRegistration{}, fmt.Errorf("Avahi publication requires IPv4")
	}
	txtValues := airPrintTXT(identity)
	txt := make([][]byte, len(txtValues))
	for i, value := range txtValues {
		txt[i] = []byte(value)
	}
	return avahiRegistration{
		Interface:   int32(ifi.Index),
		Protocol:    avahiProtoIPv4,
		Flags:       avahiPublishFlags,
		Name:        instance,
		ServiceType: "_ipp._tcp",
		Domain:      "local",
		Host:        identity.Host,
		Address:     v4.String(),
		Port:        uint16(identity.Port),
		Subtype:     "_universal._sub._ipp._tcp",
		TXT:         txt,
	}, nil
}

func runAvahi(ctx context.Context, cfg Config, instance string, ifi *net.Interface, ip net.IP) error {
	registration, err := makeAvahiRegistration(cfg.Identity, instance, ifi, ip)
	if err != nil {
		return err
	}
	address := cfg.DBusAddress
	if address == "" {
		address = defaultAvahiDBusAddress
	}

	conn, err := dbus.Connect(address)
	if err != nil {
		return fmt.Errorf("connect to Avahi system bus: %w", err)
	}
	defer conn.Close()

	server := conn.Object(avahiService, avahiServerPath)
	setupCtx, cancelSetup := context.WithTimeout(ctx, avahiRegistrationWait)
	defer cancelSetup()

	var groupPath dbus.ObjectPath
	if err := server.CallWithContext(setupCtx, avahiServerInterface+".EntryGroupNew", 0).Store(&groupPath); err != nil {
		return fmt.Errorf("create Avahi entry group: %w", err)
	}
	if !groupPath.IsValid() {
		return fmt.Errorf("Avahi returned invalid entry-group path %q", groupPath)
	}
	group := conn.Object(avahiService, groupPath)

	freeGroup := func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = group.CallWithContext(cleanupCtx, avahiEntryInterface+".Free", 0).Err
	}
	defer freeGroup()

	if call := group.CallWithContext(
		setupCtx,
		avahiEntryInterface+".AddAddress",
		0,
		registration.Interface,
		registration.Protocol,
		registration.Flags,
		registration.Host,
		registration.Address,
	); call.Err != nil {
		return fmt.Errorf("publish FolioRelay host alias through Avahi: %w", call.Err)
	}

	if call := group.CallWithContext(
		setupCtx,
		avahiEntryInterface+".AddService",
		0,
		registration.Interface,
		registration.Protocol,
		registration.Flags,
		registration.Name,
		registration.ServiceType,
		registration.Domain,
		registration.Host,
		registration.Port,
		registration.TXT,
	); call.Err != nil {
		return fmt.Errorf("publish FolioRelay IPP service through Avahi: %w", call.Err)
	}

	if call := group.CallWithContext(
		setupCtx,
		avahiEntryInterface+".AddServiceSubtype",
		0,
		registration.Interface,
		registration.Protocol,
		registration.Flags,
		registration.Name,
		registration.ServiceType,
		registration.Domain,
		registration.Subtype,
	); call.Err != nil {
		return fmt.Errorf("publish FolioRelay AirPrint subtype through Avahi: %w", call.Err)
	}

	if call := group.CallWithContext(setupCtx, avahiEntryInterface+".Commit", 0); call.Err != nil {
		return fmt.Errorf("commit FolioRelay Avahi entry group: %w", call.Err)
	}
	if err := waitForAvahiEstablished(setupCtx, group); err != nil {
		return err
	}

	ticker := time.NewTicker(avahiRegistrationHealthPoll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			state, err := avahiGroupState(ctx, group)
			if err != nil {
				return fmt.Errorf("check Avahi entry-group health: %w", err)
			}
			switch state {
			case avahiEntryEstablished, avahiEntryRegistering:
				continue
			case avahiEntryCollision:
				return fmt.Errorf("Avahi withdrew FolioRelay advertisement after name collision")
			case avahiEntryFailure:
				return fmt.Errorf("Avahi withdrew FolioRelay advertisement after failure")
			default:
				return fmt.Errorf("Avahi entry group entered unexpected state %d", state)
			}
		}
	}
}

func waitForAvahiEstablished(ctx context.Context, group dbus.BusObject) error {
	ticker := time.NewTicker(avahiRegistrationPoll)
	defer ticker.Stop()
	for {
		state, err := avahiGroupState(ctx, group)
		if err != nil {
			return fmt.Errorf("read Avahi entry-group state: %w", err)
		}
		switch state {
		case avahiEntryEstablished:
			return nil
		case avahiEntryUncommitted, avahiEntryRegistering:
			// Registration is still converging.
		case avahiEntryCollision:
			return fmt.Errorf("Avahi rejected FolioRelay advertisement after name collision")
		case avahiEntryFailure:
			return fmt.Errorf("Avahi failed FolioRelay advertisement")
		default:
			return fmt.Errorf("Avahi entry group returned unknown state %d", state)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for Avahi advertisement: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func avahiGroupState(ctx context.Context, group dbus.BusObject) (int32, error) {
	var state int32
	if err := group.CallWithContext(ctx, avahiEntryInterface+".GetState", 0).Store(&state); err != nil {
		return 0, err
	}
	return state, nil
}
