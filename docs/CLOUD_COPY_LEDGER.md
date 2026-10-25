# Cloud-copy delivery ledger

Status: source-only follow-up to cloud-copy plan #88 for #86. No worker invokes rclone or reads artifacts. No provider credentials, UI, network egress or installed App are changed.

`internal/cloudcopy.Ledger` uses the existing durable state journal. Admission validates the generated destination, binds one action ID to the exact plan and rejects changed replay. `Begin` commits `dispatching` before external I/O and returns one process-local claim; 64 concurrent callers yield one owner. The claim holder can record `accepted`, definite `failed`, or `delivery_unknown`. A restarted process projects an unfinished dispatch as `delivery_unknown`, cannot recover the claim, and cannot begin the same action again. This is intentional because the remote side may have accepted the data immediately before a crash. There is no automatic retry or automatic reconciliation of remote files.

The future worker must use a fresh authenticated action ID for any new delivery attempt after a failed or unknown result. A human-facing reconciliation workflow must inspect the exact remote object and present uncertainty rather than silently attributing an ambiguous file to a job. The ledger's `accepted` state will only mean that a separately qualified adapter reported its contractually defined acceptance result; it does not currently prove remote bytes, availability, retention, or recipient access.

Remaining gates: exact protected rclone binary; per-provider no-clobber and readback qualification; immutable canonical-artifact verification; scoped credential storage and revocation; external-worker lifecycle integration; authorized UI/API and English/Thai/German wizard. The current planner/ledger are not a cloud-output plugin and are not deployment-ready.
