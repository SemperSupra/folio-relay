# FolioRelay control runtime

This image contains only the substrate-neutral FolioRelay product authority:
`foliorelayd`, its embedded management portal/API, and the native readiness
probe. It deliberately contains no CUPS, PAPPL, shell, package manager, printer
driver, or renderer.

The TrueNAS source application should mount one persistent FolioRelay data
dataset at `/var/lib/foliorelay` and a management/ingest token file read-only
outside that dataset. The print runtime shares only the artifact handoff path
and authenticated API required by its adapter.

`compose.yaml` is a development/runtime-smoke fixture, not the TrueNAS source
application and not a substrate-selection decision.
