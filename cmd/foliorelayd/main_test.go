package main

import "testing"

func TestTLSConfigured(t *testing.T) {
	tests := []struct {
		name     string
		cert     string
		key      string
		wantTLS  bool
		wantErr  bool
	}{
		{name: "http explicit", wantTLS: false},
		{name: "https pair", cert: "/run/tls/tls.crt", key: "/run/tls/tls.key", wantTLS: true},
		{name: "certificate only", cert: "/run/tls/tls.crt", wantErr: true},
		{name: "key only", key: "/run/tls/tls.key", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tlsConfigured(tc.cert, tc.key)
			if (err != nil) != tc.wantErr {
				t.Fatalf("tlsConfigured() error = %v, wantErr %v", err, tc.wantErr)
			}
			if got != tc.wantTLS {
				t.Fatalf("tlsConfigured() = %v, want %v", got, tc.wantTLS)
			}
		})
	}
}
