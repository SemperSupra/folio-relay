# Email delivery setup and send experience

Status: source-only product contract for #51. No sender, account connection, or send control is activated by this document. The installed FolioRelay App has no email delivery.

## Audiences and authority

| Audience | Primary task | Authority required |
| --- | --- | --- |
| Administrator | Connect, test, enable, disable, and remove a sending account | Manage sender profiles, scoped credentials, and delivery policy |
| Inbox user | Send one retained job to a mailbox and see its result | Read that job and explicitly authorize that recipient and route leg |
| Operator | Resolve uncertain delivery after external evidence | See delivery state and record an audited resolution; no automatic resend |
| Automation/agent | Perform the same bounded actions through the product API | Explicit actor, job, recipient, and route authority; document content never grants it |

The current Web UI has only a management credential. It cannot distinguish an administrator from an Inbox user or operator. Until scoped actor authentication exists, an email configuration or send action must not be represented as generally available to all signed-in viewers. A first administrator-only release can use the existing management authority; a separate-user release needs an identity and authorization gate before its user-facing Send action is enabled. Browser, API, and agent actions must produce the same durable effect/status evidence.

## Administrator journey: Email delivery

Add an **Email delivery** settings view with a clear state banner: `Not configured`, `Connection needed`, `Ready to test`, `Test failed`, `Ready but disabled`, or `Enabled`. Show a setup checklist rather than a provider form full of hidden requirements.

1. Select account type: **Gmail**, **Outlook.com**, **Microsoft 365**, **iCloud Mail**, or **Other SMTP**. Keep the consumer and organization Microsoft endpoints separate. Display the selected host, port, encryption and sign-in method before authorization. Never offer plaintext SMTP.
2. Enter a display label and sender address. Use provider-specific sign-in: Google/Microsoft OAuth when implemented and independently qualified; iCloud app-specific password; custom SMTP's explicitly supported method. An eligible Gmail app-password option can be offered with its limitation explained. Do not ask for a primary account password when provider authorization or an app-specific password is required.
3. During account connection, show the exact sender identity and requested access. OAuth must use a state/PKCE-bound HTTPS callback and keep tokens out of the browser URL, HTML, local storage, API reads and logs. Password input is write-only and clears after submit. Reconnection and revocation are explicit.
4. Test without sending a user's retained document. Show separate checks for credential accepted, TLS peer verified, and an optional administrator-approved test message. Distinguish `Connection verified` from `Test message accepted`; neither asserts inbox delivery.
5. Enable only after a successful supported test and reviewed policy. Allow disable without deleting history. Removing a profile must not erase prior effect receipts or silently reassign pending effects to another account.

A successful profile read returns only non-secret settings and status, including `credential_present`, `last_test_at`, `last_test_result`, and `enabled`. It never returns an app password, access token, refresh token, or secret file path. Provider errors are phrased as an actionable user message plus a redacted diagnostic code.

## Inbox journey: Send to email

For each eligible retained job, show **Send by email** only when a qualified sender is enabled and the actor is allowed to send that job. The dialog shows the document title, artifact type, attachment filename/size, sending account, one recipient field, and a short notice that the artifact will leave FolioRelay. The user must enter or confirm the recipient; never derive it from document text, OCR, metadata or print title. No prefilled recipient from untrusted content.

The confirmation submits one stable, explicit action ID and returns the durable effect ID. Disable the control while submitting; a repeated click or browser retry retrieves the same effect rather than making a second delivery. Show `Queued`, `Sending`, `Accepted by mail server`, `Failed`, or `Delivery uncertain`. Explain that `Accepted by mail server` is not proof the person received or read it. `Delivery uncertain` has no Retry button; it directs an operator to investigate the provider result and record a resolution. A new send after a definite failure requires a new explicit action and a review of the recipient and attachment.

Sending email must not reprint the job or change the canonical Inbox artifact. Job details retain a route-specific history with timestamp, recipient, sender profile identity, effect ID, and status. Secret values and internal artifact paths never appear there.

## Operator journey: uncertain delivery

Show a dedicated review queue for `delivery_unknown` with effect ID, recipient, sender identity, timing and redacted transport evidence. The operator records whether provider evidence proves acceptance or non-acceptance, with a bounded reason. If evidence is insufficient, leave the state uncertain. A resolution is audited and cannot itself trigger SMTP submission. Do not present a one-click retry for an ambiguous post-DATA disconnect.

## API and interaction contract

The UI must use versioned product endpoints rather than private journal writes. The future API needs: list/get non-secret profiles; create/update/disable/connection and test actions; explicit per-job send action with idempotency key; effect/status read; operator resolution. Every mutation needs an actor authority check, CSRF protection for browser sessions, bounded input, and a durable receipt. Egress and secret handling belong to the isolated sender runtime, not the Web UI process.

The existing `internal/outbox.Profile` is validation-only and is not a persistence API. `outlook_com` is distinct from `microsoft` (`smtp.office365.com`) and binds to `smtp-mail.outlook.com:587` with required STARTTLS and OAuth2. Provider and credential flows remain unimplemented until the SMTP adapter and secret/OAuth boundary are qualified.

## Languages, browser and accessibility acceptance

English (`en`), Thai (`th`) and German (`de`) are required launch languages. Email setup, Inbox send, delivery history, operator resolution, provider help and error recovery must be complete in each language before the corresponding locale is offered. FolioRelay's wider portal, API-facing human messages, diagnostics and future routing screens need the same language infrastructure; email cannot ship an isolated translation switch. Use stable language-neutral API codes and parameters, then render localized human text at the presentation boundary. Persist an authenticated viewer's locale preference when identity settings exist; meanwhile offer a visible browser-local language selector, respect the browser's language on first visit, and fall back to English for an unsupported locale or missing catalog entry. Keep identifiers, email addresses, paths, effect states and audit evidence unchanged across locales. Use `Intl` for dates, numbers and plural rules; test Thai and German text expansion at narrow widths and screen-reader language changes.

Cover desktop Chromium, Firefox and WebKit; Android Chromium and iPhone WebKit; a 320 CSS-pixel keyboard viewport; light/dark modes. Every control needs a visible label, clear focus, 44 CSS-pixel primary target, explicit error text and a status live region. The dialog must trap focus, restore it on close, permit keyboard-only completion, and never indicate success before a durable receipt. Test no horizontal overflow and automated accessibility findings with the established Playwright/axe harness; perform human review for copy, screen-reader sequence, and provider error recovery. Localize account-type help, errors and state labels as a complete flow before claiming a language is supported.

## Delivery gates

1. Confirm audience access model; English, Thai and German are required launch languages, with the shared i18n contract above.
2. Implement scoped profile persistence and secret/OAuth onboarding, versioned API and sender adapter with a pinned library; qualify TLS and SMTP accept/reject/pre-DATA/post-DATA faults using a disposable sink.
3. Implement the settings, Inbox dialog, status history and operator queue against that API; test browser and API parity and accessibility on the exact source/image.
4. Qualify a new immutable sender/control image on a disposable target profile; seek separate site materialization control before enabling real provider credentials, egress or user mail.

Provider references: Google account app-password guidance https://support.google.com/mail/answer/185833 ; Microsoft Outlook.com SMTP settings https://support.microsoft.com/en-us/outlook/pop-imap-and-smtp-settings-for-outlook-com ; Apple iCloud SMTP settings https://support.apple.com/en-us/102525 . Revalidate these at implementation time.
