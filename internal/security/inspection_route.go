package security

import "fmt"

type InspectionStage string

const (
	StagePreRender   InspectionStage = "pre-render"
	StagePostRender  InspectionStage = "post-render"
	StagePreDelivery InspectionStage = "pre-delivery"
)

type InspectionClass string

const (
	InspectorLocalSocket    InspectionClass = "local-socket"
	InspectorInternalNetwork InspectionClass = "internal-network"
	InspectorExternalService InspectionClass = "external-service"
	InspectorHashReputation InspectionClass = "hash-reputation"
)

type ContentEgressMode string

const (
	EgressNone       ContentEgressMode = "none"
	EgressHashOnly   ContentEgressMode = "hash-only"
	EgressDerived    ContentEgressMode = "derived-artifact"
	EgressFull       ContentEgressMode = "full-content"
)

type InspectionRoute struct {
	InspectorID string
	Class       InspectionClass
	Stage       InspectionStage
	Egress      ContentEgressMode
}

type InspectionEgressPolicy struct {
	AllowExternal       bool
	AllowDerivedContent bool
	AllowFullContent    bool
}

func ValidateInspectionRoute(route InspectionRoute, policy InspectionEgressPolicy) error {
	if route.InspectorID == "" {
		return fmt.Errorf("%w: inspector id is required", ErrInvalidAdmission)
	}
	switch route.Class {
	case InspectorLocalSocket, InspectorInternalNetwork, InspectorExternalService, InspectorHashReputation:
	default:
		return fmt.Errorf("%w: unknown inspector class %q", ErrInvalidAdmission, route.Class)
	}
	switch route.Stage {
	case StagePreRender, StagePostRender, StagePreDelivery:
	default:
		return fmt.Errorf("%w: unknown inspection stage %q", ErrInvalidAdmission, route.Stage)
	}
	switch route.Egress {
	case EgressNone, EgressHashOnly, EgressDerived, EgressFull:
	default:
		return fmt.Errorf("%w: unknown egress mode %q", ErrInvalidAdmission, route.Egress)
	}

	if route.Class != InspectorExternalService && route.Class != InspectorHashReputation {
		return nil
	}
	if !policy.AllowExternal {
		return fmt.Errorf("%w: external inspection is disabled", ErrResourceRejected)
	}
	if route.Egress == EgressDerived && !policy.AllowDerivedContent {
		return fmt.Errorf("%w: derived-content egress is disabled", ErrResourceRejected)
	}
	if route.Egress == EgressFull && !policy.AllowFullContent {
		return fmt.Errorf("%w: full-content egress is disabled", ErrResourceRejected)
	}
	if route.Class == InspectorHashReputation && route.Egress != EgressHashOnly {
		return fmt.Errorf("%w: hash-reputation inspector may receive hashes only", ErrResourceRejected)
	}
	return nil
}
