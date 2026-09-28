# FolioRelay engineering CUPS runtime

This is a provisional, directly runnable engineering profile intended to make
the already-qualified CUPS ingress path usable while the CUPS-vs-PAPPL product
selection and full IPP Everywhere conformance work continue.

It is **not** the final production image.  In particular, the image bases are
not digest-pinned and the CUPS runtime has not yet received the final
read-only-root/supply-chain hardening pass.

## Start

From the repository root:

```sh
docker compose -f deploy/engineering-cups/compose.yaml up -d --build
```

Add the printer from a client using:

```text
ipp://HOST_RUNNING_DOCKER:8634/printers/FolioRelay
```

The engineering profile exposes only the FolioRelay printer operations on the
plain IPP port.  CUPS Web UI and browsing are disabled.  An IPPS listener is
also present on port 8635 with a self-signed CUPS certificate for qualification;
client trust/hostname policy is intentionally not automated in this profile.

Accepted artifacts are content-addressed in the `artifacts` volume and durable
FolioRelay command/idempotency state is stored in the `state-data` volume.
CUPS spool/state are separate projection volumes.

## Inspect

```sh
curl http://127.0.0.1:18080/v1/stats
docker compose -f deploy/engineering-cups/compose.yaml logs cups
```

## Stop

```sh
docker compose -f deploy/engineering-cups/compose.yaml down
```

Do not use `down -v` unless deleting the engineering state/artifact volumes is
intentional.
