# Print substrate bake-off results

Status: active evidence ledger.

## Phase 0 — build/surface feasibility

Public GitHub Actions run: `Print Substrate Bakeoff #1`, run ID
`36340940028`.

All three candidate lanes completed successfully without user intervention.

### Results

| Candidate | Functional probe | Executable bytes | Dynamic libraries | Dynamic library file bytes | Measured staged bytes | Notes |
|---|---:|---:|---:|---:|---:|---|
| CUPS 2.4.19 minimal-build baseline | build/startup probe PASS | 1,423,240 | 32 | 16,742,488 | 10,125,461 | staged tree is the upstream install tree and is not yet runtime-pruned; 372 files |
| CUPS 2.4.19 ippeveprinter-static | IPP Get-Printer-Attributes PASS | 1,639,064 | 6 | 9,437,568 | 1,639,064 | despite target name, binary is not fully static; libcups is linked statically but six system/TLS/etc. libraries remain dynamic |
| PAPPL 1.4.11 | upstream API/client/PWG Raster test set PASS | 2,557,616 | 34 | 22,611,120 | 5,370,912 | executable is upstream `testpappl`, not a FolioRelay-minimal PAPPL application; staged bytes are `libpappl.a` proxy |

### Interpretation

This phase answers **feasibility**, not production selection.

The measurements are deliberately not treated as apples-to-apples final image
sizes:

- CUPS measurement includes an unpruned upstream install tree.
- PAPPL uses the upstream test application, which intentionally enables
  functionality FolioRelay would not ship.
- ippeveprinter measurement is closest to a narrow executable, but it is an
  upstream basic/test print server and still needs production robustness
  qualification.

Current evidence supports three statements:

1. all three remain technically viable for deeper testing;
2. ippeveprinter currently has the lowest executable/runtime-library surface;
3. no candidate has yet earned production selection.

## Red-team observations from source review

### ippeveprinter command boundary

The upstream implementation invokes the configured per-job command directly
with `execve`, not via a shell. This is a useful property.

It passes IPP/job attributes through environment variables. Therefore the
FolioRelay adapter must:

- be a fixed absolute executable;
- treat every environment value as untrusted data;
- never interpolate those values into a shell command;
- never allow those values to select an executable;
- validate size/type/character bounds before creating a FolioRelay command;
- submit a structured idempotent command to the state authority;
- not directly mutate durable state.

### PAPPL test defaults are not production defaults

The upstream test application includes broad system options such as WebUI and
raw socket support because it is a framework test fixture.

A production FolioRelay PAPPL fixture must be written independently and enable
only required options. The test fixture cannot be used as the production image.

### CUPS pruning must be measured after packaging

The CUPS upstream build can turn several functions off by configuration, but a
feature being disabled by default does not mean all corresponding code or files
are absent.

The real comparison therefore requires a FolioRelay-owned final runtime image
with an explicit install manifest.

## Next evidence phases

### Phase 1 — FolioRelay ingress adapters

For each survivor:

- build a deliberately minimal server/adapter;
- submit a real print artifact;
- compute the received artifact digest;
- submit exactly one idempotent command to a test state authority;
- verify duplicate/replayed substrate callbacks do not create duplicate state
  transitions or effect intents;
- verify cancellation/status projection;
- restart substrate and adapter;
- verify FolioRelay state remains authoritative.

### Phase 2 — resource and failure pressure

- malformed IPP;
- slow/truncated clients;
- huge attributes;
- declared million-copy/page jobs;
- concurrency;
- server/adapter kill during receive and after acceptance;
- storage full;
- malformed artifact;
- downstream state authority unavailable;
- bounded backpressure.

### Phase 3 — standards/native clients

- PWG engineering conformance;
- Linux native client;
- Windows native client;
- macOS native client;
- Android emulator;
- iOS Simulator;
- DNS-SD/AirPrint/Mopria behavior where hosted networking permits.

## Selection remains gated

A permanent upstream fork is still a veto.

The eventual choice is the lowest-surface **survivor of all required gates**,
not the lowest number in this Phase 0 table.
