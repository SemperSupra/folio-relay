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
	frtls "github.com/SemperSupra/folio-relay/internal/transporttls"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:18080", "private management/ingest HTTP listen address")
	httpsListen := flag.String("https-listen", "", "external management WebUI/API HTTPS listen address; disabled when empty")
	tlsCertFile := flag.String("tls-cert-file", "", "operator-provided management TLS certificate file")
	tlsKeyFile := flag.String("tls-key-file", "", "operator-provided management TLS private-key file")
	tlsStateDir := flag.String("tls-state-dir", "", "durable directory for first-start self-signed management TLS material")
	journal := flag.String("journal", "", "durable FolioRelay journal path")
	artifactStore := flag.String("artifact-store", "", "FolioRelay content-addressed artifact store")
	tokenFile := flag.String("token-file", "", "management/ingest bearer token file")
	identityFile := flag.String("identity-file", "", "durable canonical printer identity file")
	printerURI := flag.String("printer-uri", "", "first-start canonical public IPP/IPPS URI")
	printerName := flag.String("printer-name", "FolioRelay", "first-start printer display name")
	printerLocation := flag.String("printer-location", "", "first-start printer location")
	airPrint := flag.Bool("airprint", false, "advertise AirPrint capability in the product API")
	flag.Parse()

	if *journal == "" || *artifactStore == "" || *tokenFile == "" || *identityFile == "" {
		log.Fatal("journal, artifact-store, token-file, and identity-file are required")
	}
	if *httpsListen != "" && *httpsListen == *listen {
		log.Fatal("HTTP and HTTPS listeners must use distinct addresses")
	}
	if (*tlsCertFile == "") != (*tlsKeyFile == "") {
		log.Fatal("tls-cert-file and tls-key-file must be supplied together")
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

	var tlsMaterial frtls.Material
	if *httpsListen != "" {
		now := time.Now()
		if *tlsCertFile != "" {
			fingerprint, _, err := frtls.ValidatePair(*tlsCertFile, *tlsKeyFile, identity.Host, now)
			if err != nil {
				log.Fatal(fmt.Errorf("validate operator management TLS material: %w", err))
			}
			tlsMaterial = frtls.Material{
				CertFile:          *tlsCertFile,
				KeyFile:           *tlsKeyFile,
				FingerprintSHA256: fingerprint,
			}
		} else {
			if *tlsStateDir == "" {
				log.Fatal("https-listen requires operator TLS files or tls-state-dir")
			}
			tlsMaterial, err = frtls.EnsureSelfSigned(*tlsStateDir, identity.Host, now)
			if err != nil {
				log.Fatal(fmt.Errorf("materialize management TLS: %w", err))
			}
		}
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
	handler := server.Handler()

	httpServer := &http.Server{
		Addr:              *listen,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	if *httpsListen == "" {
		log.Printf("foliorelayd private HTTP listening on %s", *listen)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(fmt.Errorf("serve HTTP: %w", err))
		}
		return
	}

	httpsServer := &http.Server{
		Addr:              *httpsListen,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	errCh := make(chan error, 2)
	go func() {
		log.Printf("foliorelayd private HTTP listening on %s", *listen)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("serve private HTTP: %w", err)
		}
	}()
	go func() {
		log.Printf(
			"foliorelayd management HTTPS listening on %s for %s certificate-sha256=%s generated=%t",
			*httpsListen, identity.Host, tlsMaterial.FingerprintSHA256, tlsMaterial.Generated,
		)
		if err := httpsServer.ListenAndServeTLS(tlsMaterial.CertFile, tlsMaterial.KeyFile); err != nil &&
			!errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("serve management HTTPS: %w", err)
		}
	}()

	log.Fatal(<-errCh)
}
