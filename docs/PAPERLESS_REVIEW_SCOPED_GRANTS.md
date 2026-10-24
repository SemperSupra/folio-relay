# Paperless review access: effect-scoped capability prototype

**Status:** DRAFT / source-only. NOT mounted in FolioRelay runtime. H0 physical HOLD; Paperless automatic filing and FolioRelay metadata PATCH disabled.

## Proven gap

The existing FolioRelay control server has a single management bearer token and HMAC session cookie. That authenticates management access but does not establish *which actor may see which Paperless filing effect*. Reusing that admin session as the filing review authorization is not acceptable.

## New scoped review authorizer

The optional Go NewScopedReviewAuthorizer helper enforces a *different*, independently issued 256-bit bearer credential, bound to exactly one filing effect SHA256 and one named actor. Grant records retain SHA-256 token digests only; they carry read permission, activation/expiration bounds and a revocation flag. A trusted request-time ReviewGrantLookup is mandatory; missing, expired, revoked, malformed or cross-record grants deny before any review metadata is loaded. A record-specific grant conveys NO right to create, update, delete, rename, route, or export files or Paperless documents. It cannot be replaced by the FolioRelay management token, a session cookie, arbitrary X-User/X-Actor/X-Forwarded-User proxy headers, or a token in query parameters.

The code intentionally supplies **no grant issuer, secret persistence, recovery, account directory or default resolver**, and does not register routes or add a listening socket. The test fixture credential is an obvious deterministic value and MUST NEVER be used outside tests. An admitted provider must securely generate cryptographically random 32-byte credentials, deliver them over HTTPS, store only necessary hashes/scope state, support immediate revocation, ensure no role confusion, persist audit events without raw tokens, and prevent user-to-user document disclosure.

### Important browser limitation

Native HTML navigation and ordinary links **do not attach an Authorization bearer header**. Thus this primitive currently supports automated/agent HTTP clients or an explicitly trusted client, but does **not** provide a usable browser login experience. The separate human browser authentication design must exchange a validated credential for an HttpOnly, Secure, SameSite-protected, per-effect session *without leaking the bearer through URLs*, or use an independently qualified existing identity provider with an effect-scoped server-side ACL. Do not implement a query-string token, globally scoped management cookie, or proxy-header trust shortcut.

## Test evidence and remaining gates

Test cases exercise allow, deny, revocation between requests, expiry, clock window, exact effect ID, forged request headers/cookies/query, malformed bearer, shared admin token rejection, API and Thai HTML output, denial before review loader, credential and approval ID non-disclosure. Existing Go cross-platform, fuzz, integrated and disposable Paperless CI must pass at exact head.

Still unqualified: real actor enrollment, secure token/session lifecycle, passwordless/screen-reader browser flow, HTTPS/proxy trust boundaries, audit trail, rate limiting, persistent grant store and first-party FolioRelay session integration. A future deployment must also qualify Paperless-own UI login, TrueNAS storage isolation and local EN/DE/TH classification; no physical H0 release is implied.

This is a bounded security primitive, **not** a promise that reviewed HTML is live or that auto-filing can be enabled.