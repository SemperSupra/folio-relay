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
	frstate "github.com/SemperSupra/folio-relay/internal/state"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:18080", "management/ingest listen address")
	journal := flag.String("journal", "", "durable FolioRelay journal path")
	artifactStore := flag.String("artifact-store", "", "FolioRelay content-addressed artifact store")
	tokenFile := flag.String("token-file", "", "management/ingest bearer token file")
	flag.Parse()

	if *journal == "" || *artifactStore == "" || *tokenFile == "" {
		log.Fatal("journal, artifact-store, and token-file are required")
	}
	rawToken, err := os.ReadFile(*tokenFile)
	if err != nil {
		log.Fatal(fmt.Errorf("read token file: %w", err))
	}
	token := strings.TrimSpace(string(rawToken))
	if len(token) < 16 {
		log.Fatal("token file must contain at least 16 non-whitespace bytes")
	}

	state, err := frstate.OpenDurableEngine(*journal)
	if err != nil {
		log.Fatal(fmt.Errorf("open durable state: %w", err))
	}
	defer state.Close()

	server, err := control.New(state, *artifactStore, token)
	if err != nil {
		log.Fatal(err)
	}

	httpServer := &http.Server{
		Addr:              *listen,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Printf("foliorelayd listening on %s", *listen)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(fmt.Errorf("serve: %w", err))
	}
}
