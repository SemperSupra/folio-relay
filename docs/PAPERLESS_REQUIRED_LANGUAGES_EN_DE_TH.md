# Required filing languages: English, German, Thai

**Status: mandatory qualification profile / source-only draft / physical H0 HOLD.** This supersedes the prior illustrative English/French/Spanish emphasis in product issue #58. Previous benchmark results remain immutable historical evidence, not accepted coverage of the new language set.

## Supported operations

The Paperless-ngx baseline MUST support each of `en`, `de`, `th` individually, mixed-script pages, and documents with text embedded in PDFs **and** image-only raster scans. Thai Unicode extraction/search and topical document classification are separate gates: a nonempty `tha` OCR model, the presence of Thai codepoints, and a native AI response are **not** proof of accurate Thai records filing.

Official Paperless-ngx v3.1.3 OCR settings (the Community TrueNAS catalog is currently 3.1.3):

```env
PAPERLESS_OCR_LANGUAGES=tha
PAPERLESS_OCR_LANGUAGE=eng+deu+tha
PAPERLESS_OCR_MODE=auto
```

The plural setting installs the additional Thai Tesseract traineddata inside the rootful Paperless Docker image; English and German ship with the base image. The singular setting actually enables the three language recognizers. Paperless documents increased CPU use for combined OCR. All three must be verified with `tesseract --list-langs` inside the **exact target image** before any filing test. Rootless execution is incompatible with the bootstrap installation method; an approved offline image containing the required traineddata is an alternative requiring separate image provenance/review.

The TrueNAS Community catalog's `paperless.additional_envs` configuration allows operator-configured settings; inspect actual rendered compose/config values for conflicting defaults or overridden AppConfig values before physical approval. The catalog's Paperless container is **rootful with broad filesystem permissions**, so the DMS MUST NOT mount FolioRelay's protected canonical Inbox, receipts, data store or source credentials. The connector exports a bounded verified copy through authenticated HTTPS and a durable no-replay effect. No TrueNAS installation or configuration change is included.

## Mandatory test lanes and stop conditions

1. **Tesseract environment:** exact image tag/digest; `eng`, `deu`, `tha` traineddata present and selected. Installed `tha` requires actual image/container evidence, not docs alone.
2. **Raster OCR:** 4 disposable PNG-only synthetic fixtures generated with fonts that support Thai shaping (libraqm) and German diacritics. English invoice, German Rechnung (including `März`, `Überweisung`), Thai `ใบแจ้งหนี้`, `จำนวนเงิน`, and mixed-language page. Require document API text readback, English/German anchors, and Thai character presence. Log codepoint overlap and latency without publishing user data.
3. **Quality benchmark (next):** 6 record categories × 3 required languages, at least 18 independently labeled positives plus 3 languages × contradictory/negative items, followed by >30 broader held-out records including Thai numerals/date formats and Buddhist Era dates, multi-page forms, receipts, blurred scans, mixed English/Thai company names, Thai-without-word-spaces and ambiguous German forms. Report per-language CER/character-overlap, type/tag/title/correspondent accuracy, false automatic route, abstention, adjudication cost and p95 CPU/inference latency. Thai must be evaluated separately; overall averaged accuracy cannot mask a failing language.
4. **Classification:** compare Paperless native rules, curated EN/DE/TH matches, trained Paperless ML and native Ollama-backed AI **without auto-apply**, all using the same ground-truth records. Small CPU model is a feasibility probe only; correct, non-blank *responses* are insufficient.
5. **Auto-filing policy:** stop on any false confident automated destination, metadata contamination, out-of-taxonomy tag/path creation, instruction following in document text, or unauthorized network egress. Keep review-required default until representative blind-holdout and safety review PASS. Canonical immutable records remain unchanged.

## Sources (actual API and configuration)

- Paperless v3.1.3 source: `paperless-ngx/paperless-ngx@v3.1.3` `docs/configuration.md`, `src/paperless/settings/__init__.py`, task APIs in `src/documents/views.py`.
- Official current configuration: https://docs.paperless-ngx.com/configuration/
- TrueNAS Community entry: https://apps.truenas.com/catalog/paperless-ngx_community/
- Tesseract language data: https://tesseract-ocr.github.io/tessdoc/Data-Files

**Do not claim Thai OCR or filing support as qualified until the pinned hosted container has passed actual raster OCR and the separate semantic accuracy gates.** Existing physical Foundry #291 STOP_AND_RECONCILE, consumed CREATE grants, and H1–H4 gates remain unchanged.
