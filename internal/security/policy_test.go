package security

import (
	"errors"
	"testing"
)

func TestAdmissionRejectsMillionCopyPathology(t *testing.T) {
	limits := AdmissionLimits{
		MaxBytes:         100 << 20,
		MaxPages:         10000,
		MaxCopies:        1000,
		MaxExpandedBytes: 1 << 30,
		MaxFanout:        16,
	}
	req := AdmissionRequest{
		Bytes:         1024,
		Pages:         1,
		Copies:        1_000_000,
		ExpandedBytes: 1024,
		Fanout:        1,
	}
	if err := ValidateAdmission(req, limits); !errors.Is(err, ErrResourceRejected) {
		t.Fatalf("expected resource rejection, got %v", err)
	}
}

func TestAdmissionRejectsNegativeValues(t *testing.T) {
	err := ValidateAdmission(AdmissionRequest{Pages: -1}, AdmissionLimits{})
	if !errors.Is(err, ErrInvalidAdmission) {
		t.Fatalf("expected invalid request, got %v", err)
	}
}

func TestInspectionPolicies(t *testing.T) {
	tests := []struct {
		name    string
		policy  InspectionPolicy
		verdict InspectorVerdict
		want    Action
	}{
		{"closed clean", PolicyFailClosed, VerdictClean, ActionContinue},
		{"closed unknown", PolicyFailClosed, VerdictUnknown, ActionQuarantine},
		{"bounded malicious", PolicyFailBounded, VerdictMalicious, ActionQuarantine},
		{"bounded timeout", PolicyFailBounded, VerdictTimeout, ActionHardenedRender},
		{"advisory malicious", PolicyAdvisory, VerdictMalicious, ActionAdvisory},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DecideInspection(tt.policy, tt.verdict); got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestRawPrinterLanguageRequiresExplicitRawProfile(t *testing.T) {
	if RawPrinterLanguageAllowed(TrustHardened) {
		t.Fatal("hardened profile must not allow raw printer languages")
	}
	if RawPrinterLanguageAllowed(TrustTrusted) {
		t.Fatal("trusted profile must not implicitly allow raw printer languages")
	}
	if !RawPrinterLanguageAllowed(TrustRaw) {
		t.Fatal("raw compatibility profile should allow explicitly-authorized raw mode")
	}
}
