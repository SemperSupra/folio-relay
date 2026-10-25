# Product-wide FolioRelay internationalization foundation (source-only)

Authority: [FolioRelay product #85](https://github.com/SemperSupra/folio-relay/issues/85), cross-lane [program #79](https://github.com/SemperSupra/folio-relay/issues/79). Initial languages: English (`en`), German (`de`) and Thai (`th`). Date: 2026-10-25.

## Accepted architecture

One canonical language-neutral code path must feed three audiences: human UI and assistive technology, API/automation, and agents. Localized text is **presentation only**, never durable event identity, actor authorization, reconciliation, filing taxonomy, or credential policy.

This first source-only increment adds `internal/i18n` with embedded `catalog/en.json`, `de.json`, `th.json`, and typed Go APIs: `Supported`, `Normalize`, `Negotiate`, `Catalog`, `Message`, `ValidateCatalogs`. Each catalog presently has 61 keys for the existing portal and self-test diagnostic messages. Catalogs are text-only and use the same placeholders. The catalog getter returns a defensive copy; unsupported/malformed locale requests yield English, no implicit identity or permission change. Interpolation is one-pass plain text. Dynamic values must only be inserted as text nodes, never `innerHTML`.

`ValidateCatalogs` rejects absent or empty translations, placeholder drift and HTML delimiters, and is exercised in source tests. These are **candidate** human translations, not human-reviewed accepted linguistic QA or released locale support.

## Phase separation

**Stage L0 (this PR):** versioned catalog contract, negotiation/fallback, exact-key equality, placeholder checks and portable tests. No changes to the installed or source WebUI routes, login, accessible copy, server API diagnostics, egress, or native print behavior.

**Stage L1 (next):** render existing complete `login → overview → inbox → printer → diagnostics → error recovery` journeys through the catalog. Bind a single accessible language selector to the authenticated shell, initial browser preferences and explicit choice; persist by verified viewer identity **only after** scoped accounts exist. Strictly keep `document.documentElement.lang` and accessible live announcements synchronized, and format times, sizes, numbers through `Intl`. Do not imply the user has extra authorization by choosing a locale.

**Stage L2 (parallel W3–W7):** translate *complete* email, file export, SMB, Paperless review, cloud enrollment and Documents/Deliveries journeys. Error code remains stable across languages and transports, e.g. `unauthorized`, `invalid_credential`, `printer.identity`, `state.inbox_projection`. The API may carry structured parameters; browser renders bounded local strings. Never translate immutable hashes, file IDs, paths, provider object keys, job IDs, timestamps, audit state, or effect status tokens.

**Stage L3 (qualification):** human review of English/German/Thai copy; five-profile Playwright browser/keyboard/axe matrix, actual screen-reader/Thai wrap/German expansion testing, recovery-path assertions and missing-key CI. Only then mark a *complete individual user journey* localized, and only after all current and admitted journeys pass may FolioRelay advertise an initial language as fully supported. Existing English-only WebUI is still English-only and installed physical build remains unchanged.

## Risk and gates

- HTML escaping and structured untrusted params belong to the presentation boundary; no localization string is an authority or valid HTML fragment.
- Unknown incoming protocol/server error codes should remain machine-legible and show a generic localized message with safe reference code; never blindly interpolate raw upstream responses into UI copy.
- A Thai/German locale must **not** be shown as supported merely because these catalogs exist. Locales require per-journey completeness, keyboard/screen reader and human review.
- Hosted cross-platform Go qualification of this source slice does **not** qualify WebUI translated behavior, physical TrueNAS images or locale-specific workflows.
- Physical printing HIL #35 and Foundry #291 remain accepted only at their pinned original immutable images. New UX images require independent source review, exact-version virtual qualification, then fresh owner-approved site update. No physical App mutation.

## Next DLE baton

1. Inspect exact-head Go security/fuzz/real runtime jobs. Preserve any first failures and correction history.
2. Add browser-controlled locale catalog projection and a *feature-gated* complete login/overview/inbox/printer/diagnostics journey, with **independent text-only React/DOM**-compatible rendering and Playwright/axe/keyboard checks. Do not expose an incomplete language selector.
3. Track independent translated email and cloud enrollment journeys through #51/#86. Keep all object IDs/API codes and security decisions language-neutral.
4. Hand off first-failure evidence, target/profile, commit, reviewer decision, source-only non-effects and next READY action to product #85 and program #79.
