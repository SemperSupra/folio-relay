package airprint

import (
    "strings"
    "testing"
)

// This small runtime-package test does not read files outside internal/airprint,
// so it remains valid inside the minimal image build's deliberately partial
// source context. Cross-file PPD and backend parity is tested in tools/.
func TestAdvertisedPrintDataPDLRemainsPDFAndURFOnly(t *testing.T) {
    if PDL!="application/pdf,image/urf" {
        t.Fatalf("unqualified change to advertised print data MIME types: %q",PDL)
    }
    if !strings.Contains(URF,"RS300") || !strings.Contains(URF,"W8") {
        t.Fatalf("unqualified raster profile change: %q",URF)
    }
}
