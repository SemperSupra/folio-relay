package security

import (
	"errors"
	"fmt"
)

type TrustProfile string

const (
	TrustHardened TrustProfile = "hardened"
	TrustTrusted  TrustProfile = "trusted"
	TrustRaw      TrustProfile = "raw-compatibility"
)

type InspectorVerdict string

const (
	VerdictClean       InspectorVerdict = "clean"
	VerdictMalicious   InspectorVerdict = "malicious"
	VerdictSuspicious  InspectorVerdict = "suspicious"
	VerdictUnknown     InspectorVerdict = "unknown"
	VerdictError       InspectorVerdict = "error"
	VerdictTimeout     InspectorVerdict = "timeout"
	VerdictUnsupported InspectorVerdict = "unsupported"
)

type InspectionPolicy string

const (
	PolicyFailClosed  InspectionPolicy = "fail-closed"
	PolicyFailBounded InspectionPolicy = "fail-bounded"
	PolicyAdvisory    InspectionPolicy = "advisory"
)

type Action string

const (
	ActionContinue       Action = "continue"
	ActionHardenedRender Action = "hardened-render"
	ActionQuarantine     Action = "quarantine"
	ActionAdvisory       Action = "advisory"
)

type AdmissionLimits struct {
	MaxBytes         int64
	MaxPages         int64
	MaxCopies        int64
	MaxExpandedBytes int64
	MaxFanout        int64
}

type AdmissionRequest struct {
	Bytes         int64
	Pages         int64
	Copies        int64
	ExpandedBytes int64
	Fanout        int64
}

var ErrInvalidAdmission = errors.New("invalid admission request")
var ErrResourceRejected = errors.New("resource limits exceeded")

func ValidateAdmission(req AdmissionRequest, limits AdmissionLimits) error {
	values := map[string]int64{
		"bytes":          req.Bytes,
		"pages":          req.Pages,
		"copies":         req.Copies,
		"expanded_bytes": req.ExpandedBytes,
		"fanout":         req.Fanout,
	}
	for name, value := range values {
		if value < 0 {
			return fmt.Errorf("%w: %s cannot be negative", ErrInvalidAdmission, name)
		}
	}

	checks := []struct {
		name  string
		value int64
		limit int64
	}{
		{"bytes", req.Bytes, limits.MaxBytes},
		{"pages", req.Pages, limits.MaxPages},
		{"copies", req.Copies, limits.MaxCopies},
		{"expanded_bytes", req.ExpandedBytes, limits.MaxExpandedBytes},
		{"fanout", req.Fanout, limits.MaxFanout},
	}
	for _, check := range checks {
		if check.limit > 0 && check.value > check.limit {
			return fmt.Errorf("%w: %s=%d limit=%d", ErrResourceRejected, check.name, check.value, check.limit)
		}
	}
	return nil
}

func DecideInspection(policy InspectionPolicy, verdict InspectorVerdict) Action {
	switch policy {
	case PolicyAdvisory:
		if verdict == VerdictMalicious {
			return ActionAdvisory
		}
		return ActionContinue
	case PolicyFailBounded:
		switch verdict {
		case VerdictMalicious:
			return ActionQuarantine
		case VerdictSuspicious, VerdictUnknown, VerdictError, VerdictTimeout, VerdictUnsupported:
			return ActionHardenedRender
		default:
			return ActionContinue
		}
	case PolicyFailClosed:
		switch verdict {
		case VerdictClean:
			return ActionContinue
		default:
			return ActionQuarantine
		}
	default:
		return ActionQuarantine
	}
}

func RawPrinterLanguageAllowed(profile TrustProfile) bool {
	return profile == TrustRaw
}
