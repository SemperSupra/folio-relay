package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/SemperSupra/folio-relay/internal/control"
	frprinter "github.com/SemperSupra/folio-relay/internal/printer"
	frstate "github.com/SemperSupra/folio-relay/internal/state"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:18080", "management/ingest listen address")
	journal := flag.String("journal", "", "durable FolioRelay journal path")
	artifactStore := flag.String("artifact-store", "", "FolioRelay content-addressed artifact store")
	tokenFile := flag.String("token-file", "", "management/ingest bearer token file")
	identityFile := flag.String("identity-file", "", "durable canonical printer identity file")
	printerURI := flag.String("printer-uri", "", "first-start canonical public IPP/IPPS URI")
	printerName := flag.String("printer-name", "FolioRelay", "first-start printer display name")
	printerLocation := flag.String("printer-location", "", "first-start printer location")
	airPrint := flag.Bool("airprint", false, "advertise AirPrint capability in the product API")
	tlsCertFile := flag.String("tls-cert-file", "", "TLS certificate PEM file for management/WebUI HTTPS")
	tlsKeyFile := flag.String("tls-key-file", "", "TLS private key PEM file for management/WebUI HTTPS")
	flag.Parse()

	if *journal == "" || *artifactStore == "" || *tokenFile == "" || *identityFile == "" {
		log.Fatal("journal, artifact-store, token-file, and identity-file are required")
	}
	useTLS, err := tlsConfigured(*tlsCertFile, *tlsKeyFile)
	if err != nil {
		log.Fatal(err)
	}
	rawToken, err := os.ReadFile(*tokenFile)
	if err != nil {
		log.Fatal(fmt.Errorf("read token file: %w", err))
	}
	token := strings.TrimSpace(string(rawToken))
	if len(token) < 16 {
		log.Fatal("token file must contain at least 16 non-whitespace bytes")
	}

	identity, err := frprinter.LoadOrCreate(*identityFile, *printerName, *printerLocation, *printerURI)
	if err != nil {
		log.Fatal(fmt.Errorf("open canonical printer identity: %w", err))
	}

	state, err := frstate.OpenDurableEngine(*journal)
	if err != nil {
		log.Fatal(fmt.Errorf("open durable state: %w", err))
	}
	defer state.Close()

	server, err := control.New(state, *artifactStore, token, identity, control.Profiles{
		WindowsIPP: true,
		AirPrint:   *airPrint,
	})
	if err != nil {
		log.Fatal(err)
	}

	httpServer := &http.Server{
		Addr:              *listen,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	scheme := "http"
	if useTLS {
		scheme = "https"
	}
	log.Printf("foliorelayd listening on %s://%s", scheme, *listen)
	var serveErr error
	if useTLS {
		serveErr = httpServer.ListenAndServeTLS(*tlsCertFile, *tlsKeyFile)
	} else {
		serveErr = httpServer.ListenAndServe()
	}
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		log.Fatal(fmt.Errorf("serve: %w", serveErr))
	}
}

func tlsConfigured(certFile, keyFile string) (bool, error) {
	if (certFile == "") != (keyFile == "") {
		return false, errors.New("tls-cert-file and tls-key-file must be provided together")
	}
	return certFile != "", nil
}
