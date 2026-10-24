# FolioRelay WebUI identity and browser qualification

Status: candidate contract owned by issue #37.

## Purpose

The embedded WebUI is a product surface, not an implementation console. It
must remain understandable to a user who does not know that CUPS, Avahi, Go, or
TrueNAS middleware exist underneath it.

The reusable lesson from the SupraCraft Bridge/VanillaCord work is the method,
not the visual identity: start with the project's actual user job, keep one
clear product-native metaphor, define misleading negative space, and prove
objective browser/accessibility behavior with established tooling.

FolioRelay is a SemperSupra project. It does not inherit SupraCraft colors,
marks, Minecraft-adjacent motifs, or the SupraCraft organization shell.

## Project-native identity

Irreducible user job:

> Accept a document through a normal printer path, retain it durably, and make
> the resulting artifact/status easy to inspect and route.

Working visual sentence:

> A calm, trustworthy virtual printer inbox: documents arrive through familiar
> native print paths and become explicit, inspectable retained artifacts.

The UI should therefore emphasize:

- FolioRelay product identity before implementation substrate identity;
- printer endpoint and native-client readiness;
- accepted documents/inbox;
- explicit status and diagnostics;
- factual language that distinguishes configured/enabled capability from
  independently verified readiness.

Negative space:

- no CUPS administration vocabulary in ordinary user journeys;
- no generic "AI" or agent imagery;
- no copied operating-system or printer-vendor trade dress;
- no SupraCraft visual branding;
- no decorative complexity that competes with status, printer identity, or
  accepted-document tasks.

A future SemperSupra organization brand snapshot may provide shared typography,
palette, mark, or shell. Browser qualification must not wait for that authority
and must not invent it locally.

## Browser qualification baseline

The exact candidate must run in:

- desktop Chromium;
- desktop Firefox;
- desktop WebKit/Safari-family;
- Android-Chromium device context;
- iPhone/WebKit device context.

Keyboard-capable desktop profiles also exercise a 320 CSS-pixel viewport so
responsive layout and keyboard/focus behavior are measured without pretending
mobile emulation proves OS-level external-keyboard behavior.

The harness uses:

- Playwright Test for rendered journeys;
- `@axe-core/playwright` for automated accessibility findings;
- Lighthouse CI for applicable unauthenticated-surface quality budgets;
- the portable static-HCI helper qualified by Bridge and VanillaCord at
  `SupraShellScripts/github-ops-lab@4273479cbda3736a09fe9cde33279d47ac37ea89`
  only where its objective assertions fit this application.

Automated accessibility evidence is not a claim of complete WCAG 2.2 AA or
Section 508 certification.

## Product-specific acceptance

The browser suite must prove:

1. login renders and is keyboard/touch usable;
2. an invalid management credential fails closed;
3. a valid credential creates the HttpOnly browser session without copying the
   management bearer token into HTML or browser storage;
4. Overview renders live status and canonical printer endpoint;
5. Inbox, Printer, and Diagnostics are navigable in every browser/device
   profile;
6. Diagnostics executes the shared product self-test and renders structured
   results;
7. logout restores the authentication boundary;
8. primary standalone controls meet the declared 44 CSS-pixel FolioRelay
   target;
9. narrow/mobile views have no accidental page-level horizontal overflow;
10. visible heading structure, focus behavior, names, semantics, and automated
    axe checks pass;
11. system light and dark preferences remain usable.

## Evidence and promotion

A green workflow is necessary but not sufficient product-support evidence.
Retain the Playwright report/results/traces/screenshots/videos on failure and
Lighthouse output, bind them to the exact candidate revision, and inspect the
result before promotion.

Any WebUI source hardening changes the control-image identity. Therefore this
browser-qualified branch must not silently inherit the already-accepted
TrueNAS image/matrix evidence. After browser qualification, normal immutable
image publication and exact-target product requalification apply before the
physical installed-system HIL target is advanced.
