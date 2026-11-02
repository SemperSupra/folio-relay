package paperless

import (
    "context"
    "crypto/tls"
    "errors"
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"
    "time"
)

// This file uses ONLY synthetic archived records, fake policy responses,
// and in-memory HTTP test handlers. It does not register any production route,
// issue a real credential, read a TrueNAS document, or contact Paperless.
func TestLiveReviewLookupRequiresSeparateTrustAuthorities(t *testing.T) {
    source := func([32]byte) (ReviewGrant, error) { return ReviewGrant{}, ErrReviewGrant }
    stop := func(context.Context, string) (bool, error) { return false, nil }
    acl := func(context.Context, ReviewGrant) error { return nil }
    for _, tc := range []struct {
        name string
        source ReviewGrantLookup
        stop ReviewEmergencyStop
        acl ReviewCurrentEntitlement
    }{
        {"no-grant-source", nil, stop, acl},
        {"no-emergency-authority", source, nil, acl},
        {"no-current-record-ACL", source, stop, nil},
    } {
        t.Run(tc.name, func(t *testing.T) {
            if _, err := NewLiveReviewGrantLookup(tc.source, tc.stop, tc.acl);
                !errors.Is(err, ErrReviewGrant) {
                t.Fatalf("missing trust dependency accepted: %v", err)
            }
        })
    }
}

func TestLiveReviewGuardRevokesStaleActorBeforeAPIBrowserDocumentRead(t *testing.T) {
    token, hash := capabilityFixture() // deterministic TEST-ONLY bearer
    ticket, record, hints := acceptedReviewFixture()
    packet, err := BuildReviewEnvelope(ticket, record, hints, ReviewThai)
    if err != nil { t.Fatal(err) }
    at := time.Date(2026, 10, 24, 8, 30, 0, 0, time.UTC)
    now := func() time.Time { return at }
    grant := ReviewGrant{
        TokenHash: hash, ActorID: "reviewer-a", EffectID: packet.EffectID,
        ValidFrom: at.Add(-time.Minute), ExpiresAt: at.Add(time.Hour), CanRead: true,
    }
    var revokedActor, emergencyStop, emergencyUnavailable, aclUnavailable bool
    emergencyCalls, aclCalls, nativeReads := 0, 0, 0
    source := func(got [32]byte) (ReviewGrant, error) {
        if got != hash { return ReviewGrant{}, ErrReviewGrant }
        return grant, nil
    }
    live, err := NewLiveReviewGrantLookup(source,
        func(ctx context.Context, effect string) (bool, error) {
            emergencyCalls++
            if ctx.Err() != nil || effect != packet.EffectID || emergencyUnavailable {
                return false, ErrReviewGrant
            }
            return emergencyStop, nil
        },
        func(ctx context.Context, got ReviewGrant) error {
            aclCalls++
            if ctx.Err() != nil || aclUnavailable || revokedActor ||
                got.ActorID != "reviewer-a" || got.EffectID != packet.EffectID {
                return ErrReviewGrant
            }
            return nil
        })
    if err != nil { t.Fatal(err) }
    load := func(_ context.Context, effect string) (ReviewEnvelope, error) {
        nativeReads++
        if effect != packet.EffectID { return ReviewEnvelope{}, ErrReviewUnavailable }
        return packet, nil
    }
    auth, err := NewScopedReviewAuthorizer(live, now)
    if err != nil { t.Fatal(err) }
    api, err := NewReviewHandler(auth, load)
    if err != nil { t.Fatal(err) }
    browser, err := NewReviewBrowserHandler(live, load,
        "https://reviews.example.test", []byte(strings.Repeat("s", 32)), now)
    if err != nil { t.Fatal(err) }

    apiRead := func() int {
        req := httptest.NewRequest("GET",
            "https://reviews.example.test/api/filing/reviews/" + packet.EffectID, nil)
        req.TLS = &tls.ConnectionState{}
        req.Header.Set("Authorization", "Bearer " + token)
        resp := httptest.NewRecorder()
        api.ServeHTTP(resp, req)
        return resp.Code
    }
    if code := apiRead(); code != http.StatusOK {
        t.Fatalf("admitted synthetic API reviewer denied: %d", code)
    }

    exchange := httptest.NewRequest("POST",
        "https://reviews.example.test/filing/review-sessions",
        strings.NewReader("{\"effect_id\":\"" + packet.EffectID + "\"}"))
    exchange.TLS = &tls.ConnectionState{}
    exchange.Header.Set("Origin", "https://reviews.example.test")
    exchange.Header.Set("Authorization", "Bearer " + token)
    exchange.Header.Set("X-FolioRelay-Review-Session", "1")
    exchange.Header.Set("Content-Type", "application/json")
    exchangeResp := httptest.NewRecorder()
    browser.ServeHTTP(exchangeResp, exchange)
    if exchangeResp.Code != http.StatusCreated || len(exchangeResp.Result().Cookies()) != 1 {
        t.Fatalf("source-only browser session exchange denied: %d", exchangeResp.Code)
    }
    scopedCookie := exchangeResp.Result().Cookies()[0]
    browserRead := func() int {
        req := httptest.NewRequest("GET",
            "https://reviews.example.test/filing/reviews/" + packet.EffectID, nil)
        req.TLS = &tls.ConnectionState{}
        req.AddCookie(scopedCookie)
        resp := httptest.NewRecorder()
        browser.ServeHTTP(resp, req)
        return resp.Code
    }
    if code := browserRead(); code != http.StatusOK {
        t.Fatalf("admitted synthetic browser reviewer denied: %d", code)
    }
    if nativeReads != 2 { t.Fatalf("baseline document reads = %d, want 2", nativeReads) }

    // RED: revoke the external actor's RECORD permission, but do NOT touch
    // the still-unexpired bearer, encrypted grant journal, or archived record.
    // BLUE: live independent ACL denial must precede the native document GET.
    revokedActor = true
    assertDenied := func(why string) {
        t.Helper()
        before := nativeReads
        if code := apiRead(); code != http.StatusUnauthorized {
            t.Fatalf("%s: stale API access admitted: HTTP %d", why, code)
        }
        if code := browserRead(); code != http.StatusUnauthorized {
            t.Fatalf("%s: stale browser access admitted: HTTP %d", why, code)
        }
        if nativeReads != before {
            t.Fatalf("%s: loader called after failed authorization: before=%d after=%d",
                why, before, nativeReads)
        }
    }
    assertDenied("record-permission-withdrawal")

    revokedActor = false
    emergencyStop = true
    aclBefore := aclCalls
    assertDenied("independent-emergency-stop")
    if aclCalls != aclBefore { t.Fatal("emergency stop must precede record ACL read") }

    emergencyStop = false
    emergencyUnavailable = true
    assertDenied("emergency-authority-unavailable")
    emergencyUnavailable = false
    aclUnavailable = true
    assertDenied("actor-ACL-authority-unavailable")

    if emergencyCalls == 0 || aclCalls == 0 {
        t.Fatal("test did not exercise both independent authorization gates")
    }
    t.Log("SOURCE_ONLY: ACL withdrawal and emergency provider failures returned 401 before DMS GET on API and browser")
}

func TestLiveReviewLookupDeniesInvalidGrantBeforeTrustProvider(t *testing.T) {
    _, hash := capabilityFixture()
    called := 0
    source := func([32]byte) (ReviewGrant, error) {
        return ReviewGrant{TokenHash: hash, ActorID: "reviewer-a",
            EffectID: strings.Repeat("a", 64), CanRead: false}, nil
    }
    live, err := NewLiveReviewGrantLookup(source,
        func(context.Context, string) (bool, error) { called++; return false, nil },
        func(context.Context, ReviewGrant) error { called++; return nil })
    if err != nil { t.Fatal(err) }
    if _, err := live(hash); !errors.Is(err, ErrReviewGrant) || called != 0 {
        t.Fatalf("invalid grant reached trust providers: %v calls=%d", err, called)
    }
}
