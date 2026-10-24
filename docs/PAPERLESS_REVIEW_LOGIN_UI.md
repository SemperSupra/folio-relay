# English/German/Thai browser review sign-in — source-only experiment

**NOT INSTALLED / NOT ENABLED.** This is stacked on draft Paperless effect-scoped browser session PR #75 (six of six source hosted checks PASSED). Physical TrueNAS H0 remains HOLD/STOP_AND_RECONCILE.

## Practical workflow for all three audiences

- A human reviewer gets an independently issued, **record-specific** capability through a separately qualified secure channel. The user opens the optional `/filing/review-login?lang=en|de|th&effect=<64-hex-effect-id>` page and enters that capability in a password-like, labeled input. The effect hash in the URL is only a record reference, **not a bearer token**.
- A single first-party, locally embedded script intercepts submission, validates the format, clears the sensitive input, and sends the bearer in a **same-origin HTTPS Authorization header** to `POST /filing/review-sessions`. The body contains only the effect ID; requests cannot redirect or use browser storage. On success the script validates a relative `/filing/reviews/<effect>` path before navigating.
- The server-issued cookie is short-lived, encrypted, HttpOnly, Secure and SameSite=Strict, valid only for the same record's read-only HTML route. Every read rechecks the underlying grant's expiry/revocation and exact actor/effect. An approved human can then open the native Paperless UI through an operator-origin-pinned token-free link, leaving **all edits to Paperless's existing login and permissions**.
- Automation/agents continue to use their separate 256-bit scoped bearer on the machine read-only review endpoint; neither user session nor automation may execute a metadata PATCH. The existing management login does not grant access to any Paperless document automatically.

The English, German, and Thai screens use semantic headings, actual `<label>` elements, `lang`, a `role=status`/aria-live notification and typed password entry. One static JS file is embedded at build time, with strict `script-src 'self'; connect-src 'self'` CSP only on the login page; actual document-review pages remain **script-disabled**. No external fonts, packages, telemetry, analytics, browser storage or token-in-URL flow.

## What is still missing

There is **no actual issuer or trusted vault** of these grants, no identity directory, no site TLS termination configuration, no session key provisioning/rotation, no persistent revoke/audit service and no authorized deployed runtime route. These are source components only. The small interface provides a user-visible login *mechanism* for a separately supplied record credential; it is not a complete identity product. Browser accessibility on a real client (including screen readers/Thai input) must still be tested in the disposable browser lane.

A public free-tier exact-head CI run must cover Ubuntu/macOS/Windows, fuzzing, integrated FolioRelay and real Paperless 3.1.3. Go tests check localized HTML, CSP, no inline scripts, token isolation, no management cookie access and no data returned to an unauthorized user. For complete browser client behavior, add headless-browser script interaction and network assertion before treating this as an end-user feature.

No personal documents, private hardware, TrueNAS Apps, SMB/ACLs, Printer/CUPS mutation, paid cloud/model calls, egress of records, metadata PATCH or auto-filing. EN/DE/TH all mandatory; Thai classifier semantic acceptance remains blocked and manual review stays the default.