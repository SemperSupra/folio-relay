package paperless

import (
    "context"
    "errors"
)

// ErrArchivedGrantPolicy denies a review-grant operation without leaking
// whether a record, reviewer, or native Paperless document exists.
var ErrArchivedGrantPolicy = errors.New("archived document review grant not authorized")

// ReviewAccessSubject binds the requested human/automation/agent reviewer and
// independent issuer to an ACTUALLY archived, committed Paperless effect.
// Every field is from a trusted, durable Outbox ticket or validated caller
// identity; none comes from OCR text, a client-supplied document title, LLM
// suggestions, a browser identity header, or a Paperless metadata response.
type ReviewAccessSubject struct {
    Action ReviewGrantAction
    ActorID string
    OperatorID string
    EffectID string
    TargetID string
    CanonicalSHA256 string
    PolicyVersion string
    ApprovalID string
    TaskID string
    DocumentID int
}

// ReviewAccessVerdict must be returned by an INDEPENDENTLY authenticated
// authorization/identity provider with record-level custody and actor grants.
// Returning an echo from a locally permissive fake is not proof of real-world
// ownership or an acceptable production authority.
type ReviewAccessVerdict struct {
    Subject ReviewAccessSubject
    ActorIdentityVerified bool
    OperatorIdentityVerified bool
    RecordPermissionVerified bool
    Allowed bool
}

// ReviewArchivedAccessOracle must validate the operator's identity and
// action permission, intended actor identity, approved record visibility,
// underlying ApprovalID authenticity, target and document ownership at the
// time of EACH decision. Revocation also requires separate authority; no
// management-token or model-generated assertion is an acceptable substitute.
type ReviewArchivedAccessOracle func(context.Context, ReviewAccessSubject) (ReviewAccessVerdict, error)

// NewArchivedGrantDecision is a bounded, GET-free composition of durable
// Paperless filing receipt validation with a separate authorization oracle.
// It can be supplied as the mandatory ReviewGrantDecision of the *anchored*
// encrypted grant ledger. This never reads Paperless over the network, grants
// global management access, sends mail, or writes remote DMS metadata.
//
// No trusted oracle implementation, actor provisioning, grant issuance HTTP
// endpoint, or TrueNAS runtime wiring is supplied in this source-only slice.
func NewArchivedGrantDecision(outbox *Outbox, targetID string,
    oracle ReviewArchivedAccessOracle) (ReviewGrantDecision, error) {
    if outbox == nil || outbox.durable == nil ||
        !identRe.MatchString(targetID) || oracle == nil {
        return nil, ErrArchivedGrantPolicy
    }

    return func(ctx context.Context, action ReviewGrantAction,
        actor, effect, operator string) error {
        if ctx == nil || ctx.Err() != nil ||
            (action != GrantIssue && action != GrantRevoke) ||
            !identRe.MatchString(actor) || !identRe.MatchString(operator) ||
            !shaRe.MatchString(effect) {
            return ErrArchivedGrantPolicy
        }

        // First require an actually terminal archived event, not merely an
        // enqueue approval, pending/unknown task or a guessed document ID.
        ticket, err := outbox.Get(effect)
        if err != nil || ticket.ID != effect ||
            ticket.State != "archived" || ticket.DocumentID <= 0 ||
            !uuidRe.MatchString(ticket.TaskID) ||
            ticket.Intent.Validate() != nil ||
            ticket.Intent.TargetID != targetID ||
            effectID(ticket.Intent) != effect {
            return ErrArchivedGrantPolicy
        }

        requested := ReviewAccessSubject{
            Action: action, ActorID: actor, OperatorID: operator,
            EffectID: effect, TargetID: ticket.Intent.TargetID,
            CanonicalSHA256: ticket.Intent.CanonicalSHA256,
            PolicyVersion: ticket.Intent.PolicyVersion,
            ApprovalID: ticket.Intent.ApprovalID,
            TaskID: ticket.TaskID,
            DocumentID: ticket.DocumentID,
        }
        result, err := oracle(ctx, requested)
        if err != nil || ctx.Err() != nil ||
            result.Subject != requested ||
            !result.ActorIdentityVerified ||
            !result.OperatorIdentityVerified ||
            !result.RecordPermissionVerified ||
            !result.Allowed {
            return ErrArchivedGrantPolicy
        }
        return nil
    }, nil
}
