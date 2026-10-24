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


# Independently labeled record types. These examples are synthetic and are
# separate from the earlier English/French/Spanish exploratory corpus.
CLASSIFICATION_CASES = {
    "invoice": {
        "en": ["INVOICE", "Amount payable: 1200 USD"],
        "de": ["RECHNUNG", "Zahlung fällig: 1200 EUR"],
        "th": ["ใบแจ้งหนี้", "จำนวนเงินที่ต้องชำระ ๑๒๐๐ บาท"],
    },
    "insurance": {
        "en": ["INSURANCE POLICY", "Coverage begins this month"],
        "de": ["VERSICHERUNG", "Versicherungsbeginn im März"],
        "th": ["กรมธรรม์ประกันภัย", "ความคุ้มครองเริ่มเดือนนี้"],
    },
    "lease": {
        "en": ["LEASE AGREEMENT", "Monthly rental payment"],
        "de": ["MIETVERTRAG", "Miete für die Wohnung"],
        "th": ["สัญญาเช่า", "ค่าเช่าที่พักรายเดือน"],
    },
    "appointment": {
        "en": ["APPOINTMENT NOTICE", "Scheduled visit at 10:00"],
        "de": ["TERMINBESTÄTIGUNG", "Besuch um 10 Uhr"],
        "th": ["ใบนัดหมาย", "วันและเวลานัดหมายสิบโมง"],
    },
    "statement": {
        "en": ["BANK STATEMENT", "Account ending balance"],
        "de": ["KONTOAUSZUG", "Kontostand und Buchungen"],
        "th": ["รายการเดินบัญชี", "ยอดเงินคงเหลือในบัญชี"],
    },
    "registration": {
        "en": ["REGISTRATION FORM", "Enrollment for school"],
        "de": ["ANMELDUNG", "Registrierung zum Studium"],
        "th": ["ใบลงทะเบียน", "ลงทะเบียนเรียนภาคการศึกษา"],
    },
}
CLASSIFICATION_NEGATIVES = [
    ("en", "invoice", ["NOT AN INVOICE", "This is only a product advertisement"]),
    ("en", "lease", ["NO LEASE CONTRACT", "This page is not a signed agreement"]),
    ("de", "invoice", ["KEINE RECHNUNG", "Nur ein unverbindliches Beispiel"]),
    ("de", "appointment", ["KEIN TERMIN", "Diese Mitteilung bestätigt keinen Besuch"]),
    ("th", "invoice", ["ไม่ใช่ใบแจ้งหนี้", "เอกสารนี้เป็นเพียงตัวอย่างเท่านั้น"]),
    ("th", "appointment", ["ไม่มีการนัดหมาย", "ประกาศนี้ไม่ใช่ใบนัดหมาย"]),
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
    classified = []
    all_cases = []
    for kind, languages in CLASSIFICATION_CASES.items():
        for language, lines in languages.items():
            all_cases.append((kind + "-" + language, language, kind, lines, False))
    for index, (language, kind, lines) in enumerate(CLASSIFICATION_NEGATIVES):
        all_cases.append(
            ("negative-" + str(index + 1) + "-" + language,
             language, kind, lines, True)
        )
    for sample_id, language, kind, lines, negative in all_cases:
        image = Image.new("RGB", (2000, 800), "white")
        draw = ImageDraw.Draw(image)
        for row, line in enumerate(lines):
            is_thai = any("\u0e00" <= ch <= "\u0e7f" for ch in line)
            font = ImageFont.truetype(thai if is_thai else regular, 75)
            draw.text((95, 100 + row * 230), line, font=font,
                      fill="black", direction="ltr")
        image.save(out / (sample_id + ".png"), optimize=True)
        classified.append({"id": sample_id, "language": language,
                           "kind": kind, "negative": negative,
                           "file": sample_id + ".png", "lines": lines})
    (out / "classification-manifest.json").write_text(
        json.dumps(classified, ensure_ascii=False, indent=2), encoding="utf-8")
    print("Generated 4 OCR smoke + 18 classification positives + "
          "6 adversarial negatives in en,de,th")
    return 0


if __name__ == "__main__":
    sys.exit(main())
