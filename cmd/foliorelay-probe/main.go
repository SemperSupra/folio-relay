package main

import (
	"errors"
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

	if err := check(&http.Client{Timeout: *timeout}, *url); err != nil {
		fmt.Fprintln(os.Stderr, "FolioRelay readiness probe failed:", err)
		os.Exit(1)
	}
}

func check(client *http.Client, url string) error {
	if client == nil {
		return errors.New("HTTP client is required")
	}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("readiness endpoint returned HTTP %d", resp.StatusCode)
	}
	return nil
}
