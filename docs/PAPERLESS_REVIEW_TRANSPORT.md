# Source-only Paperless filing review transport

**Status: DRAFT/NOT DEPLOYED.** Issue #58; stacked on the core Paperless adapter #59 and inert review-envelope PR #69. Physical FolioRelay H0 is HOLD / STOP_AND_RECONCILE. No TrueNAS App, print, SMB, real document, token, or hosted LLM endpoint is touched.

## Three audiences and actual features

NewReviewHandler() is a read-only HTTP handler with two path shapes:

- Human/assistive browser: GET /filing/reviews/<64-lowercase-hex-effect-id> — accessible semantic HTML with a heading, status, document facts, explicit warnings, direction auto for mixed OCR titles and English/German/Thai display labels.
- Agents and automation: GET /api/filing/reviews/<64-lowercase-hex-effect-id> — machine JSON showing the same native Paperless assignment and classic suggestions, with requires_human_review=true, auto_apply_allowed=false and classification_qualified=false.
- A human approval or metadata correction is NOT implemented: there are no POST/PATCH/DELETE actions, buttons, editable metadata, model-selected paths, file rewrites or automatic filing.

Both GET paths require an injected ReviewAuthorizer to authenticate the requesting actor and grant read access to the *exact effect ID*, followed by an injected ReviewLoader. No default authorizer or loader exists: creating the handler without either is an error. There is no implicit runtime route registration or listener. Identity must come from the application's trusted session or token verifier, not client-supplied identity headers. The simulated bearer token and test header in fixtures are ONLY a denial/allow test, not a credential scheme.

## Safety boundaries

- Native OCR titles are untrusted; Go html/template escapes HTML. There is no active script/form. Headers prohibit caching, default CSP resources, framing and referrer disclosure.
- Matching effect ID, positive DMS document ID, human-review and non-autoapply flags are rechecked on load. Back-end errors don't return tokens or native document contents.
- The upstream review envelope only derives from an explicitly approved, durable archived filing effect. Uncertain external results remain HOLD and aren't published as completed documents.
- Paperless may already have assigned type/storage path on import. This screen does NOT reverse those assignments. Before deployment, separately configure a conservative DMS ingest taxonomy; never treat a warning as a reversal.
- Thai and German text remain intact as Unicode, with localized English/German/Thai labels. Mixed/unknown use English chrome. Semantic accuracy, translations of original taxonomy and Thai accessibility on real documents are unqualified.
- The interface has NO approval/metadata-write path. A later correction workflow requires explicit actor approval, a current Paperless document version/snapshot precondition, a permitted native taxonomy ID set, a durable effect, conflict/unknown-outcome adjudication and independent review. OCR or LLM text must never be interpreted as an instruction.

## Hosted qualification

Use the existing public/free-tier GitHub Actions suite at the exact PR head: Ubuntu, macOS, Windows, fuzz, integrated regression and real Paperless 3.1.3. Tests include anonymous and unscoped denial *before* any loader access; rejection of malformed IDs and POST; rejection of unsafe review packets; safe Thai/German/English HTML escaping; inaccessible backend error details; no approval-ID disclosure; explicit JSON review-required contract.

## Next baton

Integrate with the application's authenticated and audited identity/scope provider on an opt-in, local-only runtime; separately review the first mutation/metadata-correction plan; qualify realistic EN/DE/TH user-review and decision accuracy. No HIL until refreshed Foundry gates and physical H0 release authority.