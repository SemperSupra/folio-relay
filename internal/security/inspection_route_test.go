package security

import (
	"errors"
	"testing"
)

func TestExternalInspectionIsDeniedByDefault(t *testing.T) {
	route := InspectionRoute{
		InspectorID: "external-sandbox",
		Class: InspectorExternalService,
		Stage: StagePreRender,
		Egress: EgressFull,
	}
	if err := ValidateInspectionRoute(route, InspectionEgressPolicy{}); !errors.Is(err, ErrResourceRejected) {
		t.Fatalf("expected external inspection rejection, got %v", err)
	}
}

func TestHashOnlyExternalInspectionCanBeExplicitlyAllowed(t *testing.T) {
	route := InspectionRoute{
		InspectorID: "reputation",
		Class: InspectorHashReputation,
		Stage: StagePreRender,
		Egress: EgressHashOnly,
	}
	policy := InspectionEgressPolicy{AllowExternal: true}
	if err := ValidateInspectionRoute(route, policy); err != nil {
		t.Fatalf("hash-only route should be allowed: %v", err)
	}
}

func TestHashReputationCannotReceiveContent(t *testing.T) {
	route := InspectionRoute{
		InspectorID: "reputation",
		Class: InspectorHashReputation,
		Stage: StagePreRender,
		Egress: EgressFull,
	}
	policy := InspectionEgressPolicy{
		AllowExternal: true,
		AllowFullContent: true,
	}
	if err := ValidateInspectionRoute(route, policy); !errors.Is(err, ErrResourceRejected) {
		t.Fatalf("expected content egress rejection for hash-only inspector, got %v", err)
	}
}

func TestFullExternalContentRequiresExplicitOptIn(t *testing.T) {
	route := InspectionRoute{
		InspectorID: "external-cdr",
		Class: InspectorExternalService,
		Stage: StagePreRender,
		Egress: EgressFull,
	}
	policy := InspectionEgressPolicy{
		AllowExternal: true,
		AllowFullContent: true,
	}
	if err := ValidateInspectionRoute(route, policy); err != nil {
		t.Fatalf("explicitly allowed external full-content route rejected: %v", err)
	}
}
