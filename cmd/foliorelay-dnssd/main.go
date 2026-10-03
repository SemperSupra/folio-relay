package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/SemperSupra/folio-relay/internal/dnssd"
	frprinter "github.com/SemperSupra/folio-relay/internal/printer"
)

func main() {
	identityFile := flag.String("identity-file", "", "durable canonical FolioRelay printer identity")
	instance := flag.String("instance", "", "optional DNS-SD instance name; defaults to printer display name")
	interfaceName := flag.String("interface", "", "optional LAN interface override; default is route-derived")
	waitSeconds := flag.Int("identity-wait-seconds", 60, "seconds to wait for canonical identity")
	flag.Parse()

	if *identityFile == "" {
		log.Fatal("identity-file is required")
	}
	if *waitSeconds < 0 || *waitSeconds > 600 {
		log.Fatal("identity-wait-seconds must be between 0 and 600")
	}

	identity, err := waitForIdentity(*identityFile, time.Duration(*waitSeconds)*time.Second)
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("publishing AirPrint DNS-SD for %s at %s", identity.DisplayName, identity.URI())
	if err := dnssd.Run(ctx, dnssd.Config{
		Identity:  identity,
		Instance:  *instance,
		Interface: *interfaceName,
	}); err != nil {
		log.Fatal(err)
	}
}

func waitForIdentity(path string, timeout time.Duration) (frprinter.Identity, error) {
	deadline := time.Now().Add(timeout)
	for {
		identity, err := frprinter.Load(path)
		if err == nil {
			return identity, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return frprinter.Identity{}, err
		}
		if time.Now().After(deadline) {
			return frprinter.Identity{}, fmt.Errorf("printer identity did not become available: %s", path)
		}
		time.Sleep(time.Second)
	}
}
