#!/usr/bin/env python3
"""Synthetic PDF visible-text-edit oracle, never a general PDF editor.

A hidden OCR overlay could change extractable text without changing rendered
appearance; this oracle mutates the *visible drawing operator* and compares
rendered pages independently. Changes 8-char synthetic token only.
"""
import pathlib
import sys
import pikepdf

ORIGINAL=b"(DOCUMENT) Tj"
EDITED=b"(DOCUMENX) Tj"

def main():
    if len(sys.argv)!=3:
        raise SystemExit("usage: edit_visible_pdf.py SOURCE.pdf OUTPUT.pdf")
    with pikepdf.Pdf.open(sys.argv[1]) as pdf:
        if len(pdf.pages)!=1:raise SystemExit("not single-page synthetic fixture")
        page=pdf.pages[0]
        if isinstance(page.Contents,pikepdf.Array):
            raise SystemExit("ambiguous multi-stream PDF: fail closed")
        data=page.Contents.read_bytes()
        if data.count(ORIGINAL)!=1:
            raise SystemExit("synthetic visible drawing operator not unique")
        if b"/Im" in data:
            raise SystemExit("unexpected image resource in reconstructed PDF")
        page.Contents.write(data.replace(ORIGINAL,EDITED))
        pdf.save(sys.argv[2])
    with pikepdf.Pdf.open(sys.argv[2]) as pdf:
        body=pdf.pages[0].Contents.read_bytes()
        if body.count(EDITED)!=1 or ORIGINAL in body:
            raise SystemExit("visible PDF object edit not retained")
    print("PASS: visible PDF content stream text operator edited exactly once")
if __name__=="__main__":
    main()
