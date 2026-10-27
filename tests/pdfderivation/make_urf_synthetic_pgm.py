#!/usr/bin/env python3
"""Source-only synthetic Apple raster grayscale fixture; never real records."""
import pathlib
import subprocess
import sys
from PIL import Image, ImageDraw, ImageFont

def main():
    if len(sys.argv)!=2:
        raise SystemExit("usage: make_urf_synthetic_pgm.py OUTPUT.pgm")
    font_path=subprocess.check_output(
        ["fc-match","-f","%{file}","DejaVu Sans"],text=True).strip()
    if not pathlib.Path(font_path).is_file():
        raise SystemExit("font not available")
    image=Image.new("L",(2480,3508),255)  # A4 / 300 dpi
    pen=ImageDraw.Draw(image)
    face=ImageFont.truetype(font_path,105)
    for i,line in enumerate([
        "FolioRelay URF SYNTHETIC",
        "PRINT TO SEARCHABLE PDF",
        "APPLE RASTER ROUNDTRIP TEST",
    ]):
        if pen.textbbox((140,240+230*i),line,font=face)[2]>=2420:
            raise SystemExit("synthetic line clipped")
        pen.text((140,240+230*i),line,fill=0,font=face)
    image.save(sys.argv[1],format="PPM")

if __name__=="__main__":
    main()
