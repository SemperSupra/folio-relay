package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"
)

func main() {
	url := flag.String("url", "http://127.0.0.1:18080/readyz", "FolioRelay readiness URL")
	timeout := flag.Duration("timeout", 3*time.Second, "probe timeout")
	flag.Parse()

	client := &http.Client{Timeout: *timeout}
	resp, err := client.Get(*url)
	if err != nil {
		fmt.Fprintln(os.Stderr, "FolioRelay readiness probe failed:", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		fmt.Fprintf(os.Stderr, "FolioRelay readiness probe returned HTTP %d\n", resp.StatusCode)
		os.Exit(1)
	}
}
