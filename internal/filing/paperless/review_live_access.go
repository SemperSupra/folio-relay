package paperless

import (
    "context"
    "crypto/subtle"
    "time"
)

// ReviewEmergencyStop answers whether the independent emergency authority has
// quarantined the requested effect (including a globally stopped installation).
// A provider outage, stale state, or ambiguous answer must return an error.
// This is NOT a durable provider implementation.
type ReviewEmergencyStop func(context.Context, string) (blocked bool, err error)

// ReviewCurrentEntitlement checks a CURRENT independent actor/record ACL and
// approval, not the decision cached when the bearer was originally minted.
// A revoked actor, moved record, deleted entitlement, or unavailable authority
// must return an error. It must not trust request headers, OCR or management
// sessions. This callback is NOT an implementation of that authority.
type ReviewCurrentEntitlement func(context.Context, ReviewGrant) error

// NewLiveReviewGrantLookup is a SOURCE-ONLY boundary adapter for the existing
// API bearer authorizer and browser session handler. Both consume the same
// ReviewGrantLookup, so they can deny stale actors before calling the document
// loader even while a previously minted bearer is cryptographically valid.
//
// This constructor deliberately requires all THREE inputs; there is no
// permissive default, emergency bypass or fallback to management identity.
// The emergency check and current record ACL must be provisioned from trusted,
// independently governed services before ANY runtime registration. This code
// supplies no providers, routes, identity system or production entitlement.
//
// The legacy direct lookup paths remain available to historical source tests;
// they are NOT acceptable candidates for a deployed record-review route.
// ReviewGrantLookup has no request context, so each independent decision uses
// a bounded context; real deployments must separately qualify cancellation,
// availability, consistency, revocation latency and resource budgets.
func NewLiveReviewGrantLookup(source ReviewGrantLookup,
    emergency ReviewEmergencyStop,
    currentACL ReviewCurrentEntitlement) (ReviewGrantLookup, error) {
    if source == nil || emergency == nil || currentACL == nil {
        return nil, ErrReviewGrant
    }
    return func(hash [32]byte) (ReviewGrant, error) {
        grant, err := source(hash)
        if err != nil ||
            subtle.ConstantTimeCompare(hash[:], grant.TokenHash[:]) != 1 ||
            !identRe.MatchString(grant.ActorID) ||
            !shaRe.MatchString(grant.EffectID) ||
            !grant.CanRead || grant.Revoked {
            return ReviewGrant{}, ErrReviewGrant
        }
        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()

        blocked, err := emergency(ctx, grant.EffectID)
        if err != nil || ctx.Err() != nil || blocked {
            return ReviewGrant{}, ErrReviewGrant
        }
        if err := currentACL(ctx, grant); err != nil || ctx.Err() != nil {
            return ReviewGrant{}, ErrReviewGrant
        }
        return grant, nil
    }, nil
}
