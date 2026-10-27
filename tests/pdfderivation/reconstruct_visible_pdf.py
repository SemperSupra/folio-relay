#!/usr/bin/env python3
"""Synthetic OCR bounding boxes -> *visible* PDF text objects.

This test-only reconstruction drops the source raster and re-types recognized
words at approximate positions. Thus text is genuinely visible and editable,
but source fonts, layout, color and artwork are NOT faithfully reconstructed.
No claim of production-ready rendering or preservation is permitted.
"""
import csv
import pathlib
import sys

from PIL import Image
from reportlab.pdfgen import canvas

DPI = 300
MIN_CONF = 35.0


def main():
    if len(sys.argv)!=4:
        raise SystemExit("usage: reconstruct_visible_pdf.py IMAGE.png TESSERACT.tsv OUTPUT.pdf")
    source, tsv, output = sys.argv[1:]
    with Image.open(source) as image:
        width, height = image.size
    if width>5000 or height>5000 or width<100 or height<100:
        raise SystemExit("invalid bounded source image")
    scale=72/DPI
    words=[]
    with pathlib.Path(tsv).open(encoding="utf-8",newline="") as reader:
        for row in csv.DictReader(reader,delimiter="\t"):
            if row.get("level")!="5":continue
            text=(row.get("text") or "").strip()
            try:
                confidence=float(row["conf"])
                x,y,w,h=(int(row[n]) for n in ("left","top","width","height"))
            except (KeyError,ValueError) as exc:
                raise SystemExit("invalid OCR bounding box") from exc
            if confidence<MIN_CONF or not text:continue
            if min(x,y,w,h)<0 or x+w>width or y+h>height or w==0 or h==0:
                raise SystemExit("out-of-bounds OCR rectangle")
            words.append((text,x,y,w,h))
    if not words:
        raise SystemExit("no qualified visible reconstruction words")
    if not any(text.upper()=="DOCUMENT" for text,*_ in words):
        raise SystemExit("synthetic DOCUMENT edit-probe not recovered by OCR")
    pdf=canvas.Canvas(output,pagesize=(width*scale,height*scale),pageCompression=0)
    pdf.setTitle("FolioRelay Synthetic Reconstructed Text")
    pdf.setAuthor("Synthetic Test Harness")
    for text,x,y,w,h in words:
        # Approximate bounding-box baseline; do NOT claim typography parity.
        size=max(6,min(30,h*scale*0.90))
        pdf.setFont("Helvetica",size)
        pdf.drawString(x*scale,(height-y-h)*scale,text)
    pdf.showPage()
    pdf.save()
    print(f"Reconstructed {len(words)} OCR words as visible Helvetica PDF text")
if __name__=="__main__":
    main()
