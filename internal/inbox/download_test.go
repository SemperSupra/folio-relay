package inbox

import (
	"strings"
	"testing"
	"time"
)

func TestDownloadNameUsesTitleMimeAndStableSuffix(t *testing.T) {
	record := Record{
		JobID: "job-1234567890abcdef", AcceptedAt: time.Date(2026, 10, 23, 17, 0, 0, 0, time.UTC),
		SourceTitle: "../Quarterly \"Report\".pdf\r\n", MediaType: "application/pdf",
	}
	got := DownloadName(record)
	if got != "Quarterly-Report-20261023-170000-1234567890ab.pdf" {
		t.Fatalf("unexpected safe PDF name %q", got)
	}
	if strings.ContainsAny(got, "/\\\"\r\n") {
		t.Fatalf("unsafe filename %q", got)
	}
}

func TestDownloadNameFallsBackForOlderJobAndURF(t *testing.T) {
	record := Record{
		JobID: "job-abcdef1234567890", AcceptedAt: time.Date(2026, 10, 23, 17, 0, 0, 0, time.UTC),
		MediaType: "image/urf", Substrate: "cups", SubstrateJobID: "42",
	}
	if got := DownloadName(record); got != "cups-42-20261023-170000-abcdef123456.urf" {
		t.Fatalf("unexpected legacy URF name %q", got)
	}
}
