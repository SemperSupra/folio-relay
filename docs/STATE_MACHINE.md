# FolioRelay state-machine architecture

Status: normative candidate architecture.

## Core model

FolioRelay is a deterministic state-transition system with multiple actuators and
one durable state authority.

Actuators include:

- WebUI;
- HTTP/API clients;
- CLI;
- agents/MCP;
- CUPS/IPP ingress;
- renderers;
- inspectors;
- delivery workers;
- reconciliation/timer workers.

Actuators **submit commands**. They do not directly mutate durable product state.

The state authority performs:

```
current snapshot + command
        |
        v
pure transition function
        |
        +--> next snapshot
        +--> append-only event(s)
        +--> zero or more durable effect intents
```

The snapshot/event/effect-intent update is atomic.

Actual external side effects occur only after the effect intent is durable.

## Why this is the core invariant

Multiple actuators may race, retry, crash, reconnect, or replay messages.

Safety must not depend on them coordinating perfectly.

The state engine therefore provides:

- per-aggregate serialization;
- monotonic generations;
- idempotency-key replay;
- stale-generation conflict detection;
- deterministic transition validation;
- atomic state + event + effect-intent persistence;
- effect IDs stable across worker crashes/restarts;
- typed ambiguous-delivery states rather than blind retry.

## Command envelope

Every mutating command contains at least:

- command type;
- aggregate/resource ID;
- actor/principal identity;
- idempotency key;
- semantic payload digest;
- optional expected generation;
- policy/context references required by the transition.

The state engine supplies authoritative acceptance sequence/time metadata.

### Idempotency

For a given aggregate:

- same idempotency key + same semantic command => return the original result;
- same idempotency key + different semantic command => conflict;
- an unaccepted generation conflict does not consume the idempotency key.

The expected generation is concurrency metadata, not part of the semantic
command fingerprint.

## Generations

Each accepted state-changing transition increments the aggregate generation by
exactly one.

A command may provide `expected_generation`.

If it does not match the current generation, the command is rejected without a
state change or side effect.

No actuator may "last writer wins" over a safety-relevant resource.

## Pure transition rule

Transition functions are deterministic and side-effect free.

They do not:

- access the network;
- execute external programs;
- read arbitrary mutable files;
- generate randomness;
- perform delivery;
- inspect wall-clock time directly.

Time-based behavior is modeled as a timer/deadline actuator submitting a
command when a deadline is reached.

Random/external values required by a workflow are supplied as explicit command
inputs and preserved in provenance.

## Effect outbox

External actions are represented by durable effect intents.

Examples:

- send a document to a printer;
- invoke a renderer;
- invoke an inspector;
- send email;
- submit fax;
- publish a webhook.

An effect intent has a deterministic stable ID derived from its owning
aggregate/transition and effect ordinal/content identity.

Workers may claim/lease an effect, but a lease does not change its identity.

Workers cannot directly edit the owning aggregate. They submit result commands.

This is the transactional-outbox boundary:

```
state transition
     |
     +-- durable effect intent
              |
              v
         effect worker
              |
              v
       result command
              |
              v
        state transition
```

## External delivery and ambiguity

Exactly-once delivery cannot be assumed for arbitrary external systems.

Therefore FolioRelay targets **effectively-once orchestration**:

- stable delivery/effect IDs;
- downstream idempotency keys when supported;
- reconciliation against backend job/message IDs when supported;
- no automatic duplicate submission after an ambiguous handoff.

A delivery that may have crossed the external boundary but lacks a definitive
result enters `delivery_unknown`.

`delivery_unknown` is a safety trap:

- it may reconcile to `delivered`;
- it may reconcile to a retryable/failed state only with evidence;
- it may be manually resolved under policy;
- it MUST NOT automatically transition back to submit/retry.

## State machines that are normative

### Artifact processing

```
quarantined
  -> inspecting
  -> render_pending
  -> rendering
  -> validating
  -> ready

failure terminals/holds:
  policy_rejected
  malformed
  resource_rejected
  quarantined
```

An untrusted artifact cannot become `ready` unless the configured trust
profile's required inspection/render/validation gates have succeeded.

The immutable original is never overwritten. Every transform creates a derived
artifact with a new digest and lineage.

### Job orchestration

```
submitted
  -> admitted
  -> active
  -> completed | partially_completed | failed | cancelled

submitted
  -> policy_rejected | resource_rejected | malformed
```

Job terminal state is derived from route-leg outcomes where possible rather
than independently invented.

### Route leg

```
pending
  -> preparing
  -> ready
  -> dispatching
  -> delivered | failed | delivery_unknown | blocked | cancelled
```

Allowed recovery:

- `failed -> pending` only via explicit retry-policy command;
- `blocked -> pending` only after the blocking condition is resolved;
- `delivery_unknown` never automatically retries.

Successful route legs are never replayed because another leg failed.

### Effect/delivery attempt

```
pending -> leased -> executing
                    -> succeeded
                    -> failed
                    -> unknown
```

Lease expiry before an external submission boundary may return an effect to
`pending`.

After an ambiguous external submission it becomes `unknown`, not `pending`.

### Destination trust

```
discovered -> observed -> approved -> active
                              |
                              +-> disabled
active -> drifted -> approved
active -> revoked
```

A discovered destination cannot receive user work before approval/policy
permits it.

Identity/address/certificate/capability drift can move an active destination to
`drifted`, which blocks unsafe delivery until reconciled/reapproved.

### Plugin qualification

```
candidate -> qualifying -> qualified -> active
     |            |             |
     +----------> rejected      +-> quarantined/revoked
```

Only active, qualified plugin identities/digests may execute production effects.

A changed plugin image/digest is a new candidate and does not inherit
qualification solely from its name/tag.

### Inspector result

Inspector verdicts are evidence, not authority:

```
requested -> running -> clean | malicious | suspicious | unknown | error | timeout
```

The policy engine maps a verdict to a transition such as continue,
hardened-render, quarantine, or advisory. An inspector cannot directly mark an
artifact `ready` or alter routes.

## System-wide safety invariants

1. **Single state authority** — durable product state changes only through the
   state engine.
2. **No side effects in transitions** — external effects are durable intents.
3. **Monotonic generations** — no rollback/last-writer-wins mutation.
4. **Command idempotence** — accepted command replay is deterministic.
5. **Immutable artifacts** — content-addressed bytes never mutate in place.
6. **No untrusted promotion** — required trust gates precede `ready`.
7. **No blind ambiguous retry** — unknown external delivery cannot auto-resubmit.
8. **No unapproved destination delivery** — destination policy/trust gate is
   checked in the transition that creates a delivery effect.
9. **Qualified plugins only** — effect creation binds to an immutable qualified
   plugin identity.
10. **Authority subset** — workers cannot use effect execution to grant
    themselves new routes, mounts, credentials, or plugin authority.
11. **Terminal monotonicity** — delivered/succeeded effects are never
    automatically re-executed.
12. **Durable-before-effect** — no worker sees an effect until its owning state
    transition is committed.
13. **Result-through-command** — workers report results through state commands,
    never direct state mutation.
14. **Content is data** — document/metadata/inspector text never becomes control
    authority.
15. **Fail typed, not optimistic** — unknown/error/timeout/block conditions are
    distinct from success.

## Persistence implementation

The logical model does not require every actuator to share a database.

The minimum architecture is one active state-authority process over an external
durable store. All actuators communicate with that authority over the common
control protocol or a local narrow RPC/Unix-socket surface.

The first implementation should optimize for correctness and observability
before HA.

Requirements:

- crash-safe write-ahead/journal semantics;
- atomic state/event/effect-intent commit;
- fsync/rename or transactional durability appropriate to the selected store;
- snapshots/checkpoints may accelerate recovery but never replace the durable
  journal;
- only one active writer unless a later HA design introduces a qualified
  consensus/lease mechanism.

Do not introduce distributed consensus into the first release.

## Durable journal and single-writer enforcement

The first production state store should use one authoritative append-only journal
plus derived snapshots/checkpoints.

The journal record contains enough information to replay:

- accepted command identity/fingerprint;
- previous and resulting generation;
- transition/event data;
- effect intents created by the transition;
- integrity/version metadata.

Requirements:

- framed records with length/version/checksum so a torn tail is detectable;
- fsync before acknowledging an accepted mutating command;
- invalid/incomplete crash tail is discarded or quarantined on recovery;
- snapshots are optimization only and can be rebuilt from the journal;
- effect intents are reconstructed from the same committed record, so an effect
  cannot exist without the state transition that authorized it;
- compaction/checkpointing preserves audit/provenance and idempotency keys for
  their required retention window.

The state-authority persistent store must support the durability primitives we
qualify. For the first release this means a local POSIX-like durable volume,
such as a TrueNAS ixVolume or Docker/Podman local volume, rather than assuming
arbitrary SMB/NFS semantics for the state journal.

Large document/artifact storage may use other qualified storage backends; the
small control-state journal does not need to share their consistency model.

### Active writer lease

Exactly one state-authority instance may write a journal.

At startup it acquires an exclusive store/writer lease before accepting
commands. Failure to establish exclusive authority is a fail-closed startup
error.

This lease prevents accidental dual writers; it is not a distributed consensus
algorithm.

The first release therefore does **not** support active-active state-authority
replicas. If high availability later earns its keep, the persistence backend
must be replaced/extended with a transactional/consensus mechanism whose
semantics are separately modeled and qualified.

## Formal verification strategy

Safety-critical transitions should have three layers of evidence:

1. machine-readable state/transition contract;
2. Go transition/property/fuzz tests;
3. model checking for concurrency/idempotence/ambiguous-effect properties.

TLA+ (or an equivalent model checker) is appropriate for:

- concurrent actuators;
- command replay;
- generation conflicts;
- worker crash/restart;
- effect lease expiry;
- ambiguous external delivery;
- recovery after state-authority restart.

The implementation must conform to the model; the model does not replace
runtime tests.
