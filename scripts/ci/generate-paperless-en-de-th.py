#!/usr/bin/env python3
"""Generate disposable raster-only English/German/Thai Paperless OCR fixtures.

No personal records, stored fonts, external text, image models or sample PDFs.
Ubuntu free-tier CI installs system Noto fonts/Pillow before running this.
"""
import json
import os
import pathlib
import subprocess
import sys

from PIL import Image, ImageDraw, ImageFont, features

FIXTURES = [
    {"id": "english", "language": "en", "lines": [
        "INVOICE", "Payment due: 1200 USD", "Document reference A-2026"
    ]},
    {"id": "german", "language": "de", "lines": [
        "RECHNUNG", "Zahlungsziel März 2026", "Überweisung und Prüfung"
    ]},
    {"id": "thai", "language": "th", "lines": [
        "ใบแจ้งหนี้", "จำนวนเงิน ๑๒๐๐ บาท", "หมายเลขเอกสาร ๒๐๒๖"
    ]},
    {"id": "mixed", "language": "en+de+th", "lines": [
        "INVOICE und RECHNUNG", "ใบแจ้งหนี้ ชำระเงิน บาท", "Überweisung payment due"
    ]},
]


def font_path(lang: str) -> str:
    p = subprocess.check_output(
        ["fc-match", "-f", "%{file}", f":lang={lang}"], text=True
    ).strip()
    if not pathlib.Path(p).is_file():
        raise RuntimeError(f"No system font file for language {lang}")
    return p


def main() -> int:
    if not features.check("raqm"):
        raise RuntimeError("Pillow requires libraqm for combining Thai shaping")
    out = pathlib.Path(sys.argv[1]).resolve()
    out.mkdir(parents=True, exist_ok=True)
    regular = font_path("de")
    thai = font_path("th")
    report = []
    for item in FIXTURES:
        image = Image.new("RGB", (2000, 800), "white")
        draw = ImageDraw.Draw(image)
        for i, line in enumerate(item["lines"]):
            is_thai = any("\u0e00" <= ch <= "\u0e7f" for ch in line)
            font = ImageFont.truetype(thai if is_thai else regular, 75)
            draw.text((95, 70 + i * 220), line, font=font, fill="black",
                      direction="ltr")
        path = out / (item["id"] + ".png")
        image.save(path, optimize=True)
        report.append({"id": item["id"], "language": item["language"],
                       "file": path.name, "lines": item["lines"]})
    (out / "manifest.json").write_text(
        json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
    print("Generated 4 synthetic raster-only OCR fixtures; languages=en,de,th,mixed")
    return 0


if __name__ == "__main__":
    sys.exit(main())
