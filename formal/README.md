# Formal state models

FolioRelay uses small formal models for concurrency/safety properties that are
easy to state incorrectly in ordinary code tests.

Current models:

- `CommandIdempotency.tla` — repeated/reordered commands cannot create extra
  generation advances or duplicate effect intents;
- `DeliveryAmbiguity.tla` — an ambiguous external delivery cannot blindly
  return to a retryable state.

TLC is pinned by content digest in CI.

These models are intentionally smaller than the implementation. They define
safety properties the Go state engine and black-box qualification must refine.

Future models should cover:

- state-authority crash/recovery and journal tail truncation;
- effect lease expiry before/after an external submission boundary;
- destination identity drift;
- plugin qualification/revocation races;
- artifact promotion gates;
- concurrent cancellation versus delivery completion.
