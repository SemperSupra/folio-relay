# Human browser review: source-only session exchange

**Status DRAFT / not deployed.** Builds on the source-qualified effect-scoped grant resolver and real Paperless-ngx 3.1.3 loader in FolioRelay #74. Mandatory languages EN/DE/TH; native Paperless remains the sole DMS; document classification and FolioRelay metadata PATCH are not authorized. Physical Foundry H0 is HOLD/STOP_AND_RECONCILE.

## Authentication contract

The existing FolioRelay management bearer/HMAC cookie cannot authorize another person's record. The separately admitted read capability in #74 is a 32-byte opaque bearer tied to exactly one approved filing-effect SHA, actor, expiry and revocation record.

**POST /filing/review-sessions** is a *source-only*, opt-in TLS endpoint which exchanges that bearer in the **Authorization HTTP header**, with the effect ID in a small JSON body. It requires an exact operator-configured HTTPS public origin, a matching `Origin`, a custom `X-FolioRelay-Review-Session: 1` header, JSON-only input, correct Host and real direct TLS; forged X-Forwarded-* values are ignored. This is not a form action, login page, user credential issuer, public CORS endpoint, or deployment of the route. The response is only a relative `review_path` and a new encrypted session cookie.

A fresh random AES-GCM nonce and **separate independently supplied 32-byte key** seal the actor, exact effect ID, opaque grant hash and bounded 15-minute expiry. The browser cookie is `Secure`, `HttpOnly`, `SameSite=Strict`, host-only and scoped to `/filing/reviews/`, not the management/API routes. It never contains the raw bearer, token issuer identity, Paperless token, document content or approval ID. The cookie key must come from a protected provider, **never** the FolioRelay admin token or a GitHub/CI fixture.

**GET /filing/reviews/<effect_sha256>** decrypts only that scoped cookie, verifies the exact effect and actor, checks the session deadline, and queries the authoritative revocable grant store again **before** invoking the native Paperless GET-only document loader. Old keys, tampered/duplicate cookies, wrong actor/effect, revocation and expired grants deny without returning record details. A cookie cannot access `/api/filing/reviews` or the management endpoints, and browser GET cannot PATCH Paperless. Standard automation/agent callers continue using their independent scoped bearer review authorizer rather than inheriting browser cookies.

## Bounds and unresolved implementation

The helper `NewReviewBrowserHandler` has no default token, storage provider, key, origin or listener. It is not mounted into `cmd/foliorelayd` or TrueNAS. It consumes an injected resolver, review loader, operator-approved HTTPS origin and separate AES key. Tests use deliberately synthetic secrets. Browser users still require an independently designed trustworthy way to enter or obtain their per-effect grant in the browser; no token may be placed in links/URLs. This slice enables the exchange API, not a fully tested accessible login flow.

Source-only risks/gates: a real actor directory, high-entropy secure grant creation and distribution, durable revoke/audit store, managed session-key rotation and persistence across application restarts, rate limits, rate/credential abuse tests, trusted browser login UX, cross-origin/SameSite checks behind intended HTTPS topology, independent security review, exact free-tier hosted tests and later Foundry virtual configuration matrix. The current direct-TLS check purposely refuses proxy-terminated plaintext; an authorized proxy bridge remains separate.

Paperless native metadata assignment can be wrong on negated documents and Thai has no qualified automatic document-type classification. Do not enable auto-apply or FolioRelay-issued metadata PATCH. Paperless 3.1.3 ignores stale `If-Match` on metadata PATCH and the second writer wins; native Paperless UI remains authoritative for manual corrections.

No physical TrueNAS installation, private records, CUPS, SMB/ACL, model egress, paid resource or HIL occurred.