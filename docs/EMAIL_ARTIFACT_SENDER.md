# Email artifact sender

Status: candidate FolioRelay sender plugin.

ABI:

`foliorelay.sender.email-artifact/v1`

## User job

Email the document artifact associated with a print/routing job to one
user-specified mailbox **in addition to** the job's ordinary route legs.

Example:

```
document
  -> normalize
  -> route leg: office printer
  -> route leg: archive
  -> route leg: email-artifact -> person@example.com
```

Each leg has independent state and retry semantics. Failure of the email leg
does not replay an already successful print/archive leg.

## Which artifact is emailed

The default attachment is the **canonical page-faithful normalized artifact**
that represents what the user intended to print.

It is not:

- raw PJL/PCL/PostScript;
- a printer-specific spool payload;
- an untrusted original when the route required hardened normalization;
- a temporary renderer scratch file.

For a hardened/untrusted route, the emailed artifact must come from the same
approved derived-artifact lineage that passed the route's security policy.

A route may later select another explicitly qualified derived artifact, but the
sender never chooses an arbitrary source file on its own.

## Recipient authority

The recipient is a structured control-plane field supplied by an authorized
human/API/agent command.

The recipient MUST NOT be inferred from:

- document body text;
- OCR;
- PDF metadata;
- filename;
- QR/barcode content;
- email-like strings discovered inside the document;
- inspector output.

Document content cannot grant external-delivery authority.

Initial scope is exactly one user-specified recipient per email route leg.
Multiple recipients can be represented as multiple independent route legs,
which preserves explicit provenance and per-recipient delivery state.

Deployments may additionally constrain allowed recipient domains/addresses.

## Effect identity and idempotence

The email route leg creates a durable delivery effect containing:

- immutable artifact digest;
- normalized recipient identity;
- sender/plugin immutable identity;
- message/subject policy;
- route-leg ID.

The effect ID is stable across process/runtime replacement.

If the SMTP/provider supports an idempotency key, FolioRelay supplies the stable
effect ID. SMTP itself does not guarantee exactly-once delivery, so ambiguous
handoff enters `delivery_unknown` and MUST NOT blindly resubmit.

Successful print/archive legs remain successful regardless of the email leg's
outcome.

## Message shape

Default:

- one attachment;
- attachment filename derived from trusted route/job metadata, sanitized for
  MIME/header use;
- MIME type from the approved artifact;
- concise FolioRelay-generated subject/body;
- no document body/metadata copied into mail headers without explicit escaping
  and policy.

The sender may include stable job/route identifiers for correlation, but must
not expose secrets or sensitive internal paths.

## Security boundary

The real email sender runs non-root and receives only:

- the email outbox/delivery effect;
- read-only access/stream access to the selected artifact;
- its own scoped SMTP/provider credential;
- network egress to the configured mail endpoint.

It does not receive:

- CUPS admin authority;
- printer networks;
- other sender credentials;
- general document-store write access;
- route-management authority.

The null email sender remains networkless and exists for contract/testing.

## Provider model

The sender ABI is transport-neutral.

Potential implementations:

- SMTP + STARTTLS/TLS;
- authenticated submission relay;
- provider API adapter when justified.

SMTP is the first generic target. Provider-specific adapters are optional and
must earn their dependency/credential surface.

## Address handling

The core treats the recipient as one mailbox, never as a raw header fragment.

Requirements:

- reject CR/LF/NUL/control-character injection;
- reject recipient lists in a single-recipient field;
- normalize domain representation where safe;
- preserve the user's mailbox semantics;
- support internationalized addresses only when the selected sender/provider
  explicitly advertises the required SMTPUTF8/EAI capability.

An unsupported internationalized address fails as a typed capability error
rather than being silently mangled.

## State behavior

The email leg uses the common route-leg/effect state machines:

```
pending
 -> preparing
 -> ready
 -> dispatching
 -> delivered | failed | delivery_unknown | blocked
```

Examples:

- invalid/unauthorized recipient -> policy-rejected before effect creation;
- mail endpoint unavailable before submission -> failed/retryable by policy;
- provider definitively rejects recipient -> failed;
- connection drops after DATA/provider submit where acceptance is ambiguous ->
  delivery_unknown;
- successful provider acceptance -> delivered according to the sender's
  qualified delivery semantics.

"Delivered" means accepted by the configured email transport/provider, not that
a human read the message.

## Fan-out semantics

Adding email does not make the print route transactional with email.

For:

```
printer + archive + email
```

the valid result can be:

```
printer: delivered
archive: delivered
email: failed

job: partially_completed
```

Retrying email does not reprint or rearchive.

A future policy may require all route legs for an overall workflow objective,
but that requirement still does not justify replaying already successful
external effects.
