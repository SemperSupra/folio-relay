#!/usr/bin/env python3
"""Synthetic-file XMP and Info *readback* proof; not a runtime PDF writer.

Do not use open_metadata() to create missing metadata and then mistake that
new metadata for proof that the input file already contained embedded XMP.
"""
import sys
import pikepdf

if len(sys.argv) != 4:
    raise SystemExit("usage: check_embedded_metadata.py PDF EXPECTED_TITLE EXPECTED_AUTHOR")

path, expected_title, expected_author = sys.argv[1:]
with pikepdf.open(path) as document:
    if "/Metadata" not in document.Root:
        raise SystemExit("FAIL: finished PDF has no embedded XMP stream")
    if str(document.docinfo.get("/Title", "")) != expected_title:
        raise SystemExit("FAIL: PDF Info Title mismatch")
    if str(document.docinfo.get("/Author", "")) != expected_author:
        raise SystemExit("FAIL: PDF Info Author mismatch")
    # Disable automatic Info -> XMP synchronization so this readback cannot
    # silently synthesize the requested XMP keys from PDF Info fields.
    metadata = document.open_metadata(
        set_pikepdf_as_editor=False,
        update_docinfo=False,
        strict=True,
    )
    title = metadata.get("dc:title")
    if str(title) != expected_title:
        raise SystemExit(f"FAIL: embedded XMP dc:title mismatch: {title!r}")
    author = metadata.get("dc:creator")
    if expected_author not in str(author):
        raise SystemExit(f"FAIL: embedded XMP dc:creator mismatch: {author!r}")
print("PASS: raw PDF has embedded XMP and independently read Info + XMP title/author")
