# Managed export: hostile group-share writer adversarial qualification

Status: source-only, draft; refs [FolioRelay #50](https://github.com/SemperSupra/folio-relay/issues/50) and integrated [DLE #79](https://github.com/SemperSupra/folio-relay/issues/79). Stacks on existing [draft export safety correction #57](https://github.com/SemperSupra/folio-relay/pull/57).

## Observed boundary
The worker copies verified immutable canonical artifacts to files underneath a separately protected export parent. A private `.foliorelay-private-stage/` directory (mode 0700) is a sibling of the group-writable `shared/` subtree (mode 0770), not a directory controlled by SMB members. Publication uses same-filesystem exclusive hardlink and must not overwrite preexisting destinations.

## Adversarial fixture
`TestHostileShareWriterCannotPublishOutsideOrTouchPrivateStage` uses synthetic source bytes, an isolated mode-0660 export profile and an untrusted concurrent actor that injects/removes destination symlinks in `shared/` while the worker attempts publication. It checks:

- A preexisting symlink pointing outside the protected export root causes `ErrCollision` instead of following/overwriting it.
- A concurrent SMB-style writer may change or remove exported copies, but cannot cause modifications to the original canonical source or an outside sentinel.
- The staging directory remains mode 0700 and empty after publication attempts.
- No test grants permissions to use `shared/` as the worker's staging parent, changes TrueNAS ACLs, uses real SMB credentials, or exercises live services.

## Limits / falsification scope
The simulated attacker shares the same CI process uid, so this only exercises path-safety invariants and adversarial file operations. It **does not qualify** actual separate-account permission enforcement, group inheritance, ACLs, Samba options, TOCTOU behavior on real ZFS, Windows filesystem semantics, or multiple users. A concurrently deleted output is not a durable delivery: the canonical Inbox/effect ledger remains authoritative, and re-export needs a new authorized effect.

The test is POSIX-only. Windows mode 0660 must remain denied pending Win32 ACL-specific implementation and qualification.

## Next gate
Exact-head public hosted Go tests/race/fuzz and integrated workflow on this draft; log first failures. Then review the whole worker/parent ownership and symlink/race implications; implement named SMB allow/deny tests on a disposable exact-version TrueNAS with independently approved site profile. Only after source/security review and F0–F5 may an owner authorize physical export share changes. Historical printing H0–H4 was already accepted and is not export permission.
