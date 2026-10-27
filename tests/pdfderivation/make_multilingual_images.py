#!/usr/bin/env python3
"""Disposable multilingual raster fixtures; no real personal document content.

Generates synthetic single-page EN/DE/TH images and authoritative ground truth.
Uses Pillow's RAQM text shaping for Thai and fontconfig-selected OS font;
fails closed rather than accidentally rendering boxes or Latin substitute glyphs.
"""
import json
import pathlib
import subprocess
import sys
import unicodedata

from PIL import Image, ImageDraw, ImageFont, features

SAMPLES = {
    "eng": {
        "font": "DejaVu Sans",
        "title": "FolioRelay Synthetic English",
        "lines": ["FolioRelay Synthetic Receipt", "DOCUMENT SAMPLE TEST", "INVOICE AMOUNT 125"],
        "probe": "SYNTHETIC",
    },
    "deu": {
        "font": "DejaVu Sans",
        "title": "FolioRelay Synthetisches Deutsch",
        "lines": ["FolioRelay DEUTSCH", "RECHNUNG UND BETRAG", "ÜBUNG GRÖSSE ÄNDERUNG"],
        "probe": "RECHNUNG",
    },
    "tha": {
        "font": "Garuda",
        "title": "FolioRelay ทดสอบภาษาไทย",
        "lines": ["ภาษาไทย ทดสอบ", "เอกสาร ประเทศไทย", "ทดสอบ เอกสาร ภาษาไทย"],
        "probe": "ภาษาไทย",
    },
}


def font_file(name):
    return subprocess.check_output(
        ["fc-match", "-f", "%{file}", name], text=True
    ).strip()


def render(output_dir, code, data):
    path = font_file(data["font"])
    if not pathlib.Path(path).is_file():
        raise RuntimeError("missing qualified font for " + code)
    # Require complex-script shaping support; a plain unshaped image is not
    # valid evidence of printed Thai characters.
    if code == "tha" and not features.check("raqm"):
        raise RuntimeError("Pillow lacks RAQM Thai shaping")
    font = ImageFont.truetype(path, 90, layout_engine=ImageFont.Layout.RAQM)
    image = Image.new("RGB", (2000, 2400), "white")
    pen = ImageDraw.Draw(image)
    for n, text in enumerate(data["lines"]):
        text = unicodedata.normalize("NFC", text)
        bbox = pen.textbbox((130, 200 + 205*n), text, font=font)
        if bbox[2] >= 1870 or bbox[3] >= 2300:
            raise RuntimeError("synthetic line clipped, invalid OCR oracle")
        pen.text((130, 200 + 205*n), text, font=font, fill="black",
                 language="th" if code == "tha" else "en")
    outfile = output_dir / f"{code}-raster.png"
    image.save(outfile)
    return {"lang": code, "lines": data["lines"], "probe": data["probe"],
            "font_name": data["font"], "font_file": path, "title": data["title"]}


def main():
    if len(sys.argv) != 2:
        raise SystemExit("usage: make_multilingual_images.py OUT_DIR")
    out = pathlib.Path(sys.argv[1])
    out.mkdir(parents=True, exist_ok=True)
    manifest = [render(out, code, data) for code, data in SAMPLES.items()]
    (out / "ground-truth.json").write_text(
        json.dumps(manifest, ensure_ascii=False, indent=2) + "\n",
        encoding="utf-8",
    )
    print("Synthetic only, no private documents: " +
          ", ".join(entry["lang"] for entry in manifest))


if __name__ == "__main__":
    main()
