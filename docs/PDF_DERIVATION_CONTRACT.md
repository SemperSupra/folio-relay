# FolioRelay virtual Print-to-PDF normalization — source contract & research qualification

**Product authority:** [#104](https://github.com/SemperSupra/folio-relay/issues/104) (2026-10-27). Links: accepted installed virtual printer HIL #35; Documents collection #87; account + per-document access #96; content-aware filing #58. This is a source-only proposal. The currently installed FolioRelay captures PDF/URF, it **does not produce a normalized searchable/editable PDF**.

## Expected user experience

Every supported app with a functioning OS print interface offers a **FolioRelay** virtual printer. A print submits PDF/URF *print data*, not a guaranteed Office-native editable document. FolioRelay preserves the immutable original and derives a separately addressable **.pdf** user document containing embedded Unicode text where extractable or OCR-recognizable, verified embedded metadata, and explicitly qualified editability.

### Precise editability tiers

| Tier | When available | What user may do |
| --- | --- | --- |
| `native-editable` | Born-digital PDF retains visible text/vector objects and a visible-text edit has been demonstrated | Search, select, and edit actual retained PDF text objects using compatible PDF editor (does not imply DOCX, Excel formulas, native app layers, etc.) |
| `reconstructed-editable` | Independent layout reconstruction creates **visible editable text objects**, verified by actual edit/re-render | Edit reconstructed text objects; layout, fonts, tables and links may differ; confidence + exceptions visible |
| `searchable-image` | Raster URF/scanned PDF is OCRed and a searchable/selection layer embedded over image | Search and copy words; **not** described as genuinely editable page text |

OCRmyPDF/Tesseract can make images searchable, but OCR-invisible text by itself cannot satisfy reconstructed editability. We must not force-rasterize born-digital text: OCRmyPDF `--mode force` destroys original text/structure. `--mode skip` retains text pages but PDF/A conversion may still discard tagged structure via Ghostscript; keep regular PDF whenever preservation/edits take precedence. [OCRmyPDF advanced](https://ocrmypdf.readthedocs.io/en/stable/advanced.html). PDF/A is a separately validated archival variant when requested, not a synonym for searchable, editable, or accessible.

## Metadata profile

Default embedded standard Info/XMP summary: title, creator/tool/profile version, creation/modification time, language(s), keywords/subject when evidence-backed, optional author **only if actually provided or verified**, original print job ID and artifact digest in a provenance manifest. Use versioned namespaces, avoid exposing machine hostname, full local filesystem path, network or credential fields in a publicly exported PDF. Distinguish app-provided author/title vs OCR suggested metadata, and never treat guesses as an actual author. Validate embedded readback *from finished bytes*. An external JSON sidecar may hold provenance/OCR confidence but is not a substitute for embedded PDF metadata.

## Pure-Go contract

`internal/pdfderivation/contract.go` declares immutable original source MIME + SHA-256, derived PDF identity, state, versioned processor profile, editability and text origin, metadata and verifiable output evidence. `Receipt.ValidateReady` rejects:
- non-PDF derived output, unsupported source, invalid digest, zero pages, missing processor profile or non-ready status;
- output absent PDF parsing, page parity, extractable Unicode, or Info/XMP metadata readback;
- `native-editable` when source was URF, text became flattened, original native text not preserved, or actual visible edit not proven;
- `reconstructed-editable` if only invisible OCR was verified rather than a visible-text modification/re-render oracle.

This Go package makes **no filesystem, network, printer or document mutation**; its evidence booleans are not themselves a trusted runtime proof. In a real service only a qualified verifier with readback provenance may set them. No authentication/authorization is conferred by receipt construction.

## Disposable OCR synthetic proof (free public hosted CI)

`.github/workflows/pdf-derivation-synthetic.yml` builds an explicitly fake English PDF fixture, renders a raster copy, runs OCRmyPDF/Tesseract CLI to produce a searchable PDF and embedded title/author metadata, and checks PDF parseability, extracted text, declared page count, PDF Info readback, and SHA-256 provenance. Also checks the original native PDF stays text-extractable through `--skip-text`. The output is explicitly `searchable-image` and is **not** proof of reconstructed visible-text editing. English + language packs installed for future EN/DE/TH fixture extension; version readbacks retained. Python ReportLab is used only for a disposable test fixture; Go remains the deployable orchestration contract and OCR engine is a separately isolated CLI. No user files or private credentials.

## Next implementation and hard gates

1. Source verify cross-platform Go/race/vet and disposable Linux headless OCR proof; inspect first failures (APT tool availability/version, OCR extraction/metadata/XMP, default no-network beyond dependency install).
2. Extend corpus to synthetic EN/DE/TH and mixed source PDF/URF (not PNG passed as URF), use independently qualified URF decoder and real comparison of pages and text; exercise low-confidence and fail-closed conditions.
3. Build isolated bounded OCR/reconstruction worker, separate immutable derivative identity `(sourceSHA,profileVersion,kind)`, crash-safe atomic writes, no network egress, resources measured, safe metadata provenance and readback.
4. Independent visible PDF text editor test: modify one word in output using an editor, render back, demonstrate that **the visible word changes** rather than only hidden OCR; compare normalized PDFs to sources, classify actual tier. Optionally separate DOCX/ODT structure reconstruction.
5. Integrate user-facing default PDF download, original source access, readiness/review/failure states, EN/DE/TH human/automation/agent surfaces and accessible status, without unauthorized document/user actions. Separate identity #96, provider #58, code/release/HIL gates.
6. Review licensing of OCRmyPDF (MPL-2.0), Tesseract, Ghostscript, pikepdf, alternative Stirling API, and AGPL/commercial MuPDF before embedding. Pin dependencies and scan/runtimes independently.
7. New immutable images, TrueNAS virtual F0-F5, physical new target admission **after independent review**. Historical H0-H4 and consumed Foundry deployment CREATE stay untouched.

**Status:** The source Go contract and synthetic CI workflow are research-only. No new user-facing normalized PDF capability is deployed or claimed ready.


## Additional pure-Go derivative identity and event projection (draft only)

`internal/pdfderivation/lifecycle.go` offers an immutable **intent key**, domain-separated SHA-256 over version, canonical original SHA-256 + MIME, processor profile and output kind (`searchable-pdf` or `reconstructed-editable-pdf`). The key **is not a file path** and confers no access authority. `JobEvent` supplies a proposed monotonically sequenced, idempotent append-only model with stable actions `start`, `complete`, `require_review`, `fail` and `cancel`. Identical duplicate event IDs replay without duplicate transitions; conflicting duplicates or sequence gaps fail closed. `Projection` gives humans, automation and agents a common state, reason and explicit next-action list. Completion requires the previously defined independent `Receipt.ValidateReady`; a searchable-image OCR result cannot be accepted as an editable rendition.

This is **only a reducer and contract**. It does not persist events, enforce a transaction, authenticate clients, grant record permissions, reserve resources, render PDFs, perform exports, or write a content-addressed derivative. Actual durable journal storage, crash/replay reconciliation, scoped ACL (#96), HTTP/CLI/UI surfaces and effect execution remain future separately reviewed lanes. Production authority must not trust a caller-supplied `Evidence` struct; evidence must come from independently verified bytes in the sandboxed worker.
