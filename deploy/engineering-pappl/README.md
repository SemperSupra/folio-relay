# FolioRelay engineering PAPPL runtime

This is a provisional, directly runnable engineering profile for the active
PAPPL print-substrate candidate. It exists to give PAPPL the same packaged,
least-authority qualification surface already exercised by the engineering CUPS
profile.

It is **not** a production-selection decision. The image bases and source inputs
are not yet digest-pinned, the dynamic runtime closure still needs final
supply-chain treatment, and CUPS versus PAPPL remains an evidence-driven
selection after symmetric veto gates close.

The packaged authority envelope is intentionally strict:

- state and PAPPL run as UID/GID 10001;
- root filesystems are read-only;
- all Linux capabilities are dropped;
- `no-new-privileges` is enabled;
- durable state, PAPPL spool/projection state, and accepted artifacts are
  explicit volumes;
- only bounded `/tmp` scratch is writable outside those volumes.

The PAPPL spool volume also carries
`foliorelay-substrate-instance`. That identifier is stable only for the
lifetime of the persisted PAPPL state epoch. Qualification explicitly restarts
the PAPPL process and proves that a different post-restart document receives a
new FolioRelay acceptance without an idempotency conflict. If PAPPL ever reuses
its source job identifiers across such a restart, this profile must fail rather
than silently accepting an unsafe identity mapping.

## Start

From the repository root:

```sh
docker compose -f deploy/engineering-pappl/compose.yaml up -d --build
```

Add the printer from a client using:

```text
ipp://HOST_RUNNING_DOCKER:8633/ipp/print
```

Accepted artifacts are content-addressed in the `artifacts` volume and durable
FolioRelay command/idempotency state is stored in `state-data`. PAPPL
queue/history remains disposable projection state in `pappl-state`.

## Inspect

```sh
curl http://127.0.0.1:18081/v1/stats
docker compose -f deploy/engineering-pappl/compose.yaml logs pappl
```

## Stop

```sh
docker compose -f deploy/engineering-pappl/compose.yaml down
```

Do not use `down -v` unless deleting the engineering state/artifact/PAPPL
volumes is intentional.
