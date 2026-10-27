#!/usr/bin/env python3
"""Independent UTF-8 ground-truth/readback oracle for synthetic OCR documents.

Searchable text != genuinely editable visible text. Report normalized recall
rather than claiming 100% recognition or language-independent perfection.
"""
import difflib
import json
import pathlib
import sys
import unicodedata


def normalize(value):
    value = unicodedata.normalize("NFC", value).casefold()
    return "".join(c for c in value if c.isalnum())


def main():
    if len(sys.argv) != 2:
        raise SystemExit("usage: check_multilingual_ocr.py EVIDENCE_DIR")
    root = pathlib.Path(sys.argv[1])
    expected = json.loads((root / "ground-truth.json").read_text(encoding="utf-8"))
    policy = {"eng": 0.75, "deu": 0.68, "tha": 0.55}
    results = []
    errors = []
    for item in expected:
        lang = item["lang"]
        text = (root / f"{lang}-extracted.txt").read_text(encoding="utf-8")
        source = normalize(" ".join(item["lines"]))
        observed = normalize(text)
        # Matching blocks keep order. This is approximate character recall,
        # not a word error rate nor proof of semantic correctness.
        match = difflib.SequenceMatcher(None, source, observed, autojunk=False)
        recall = sum(b.size for b in match.get_matching_blocks()) / max(1, len(source))
        probe = normalize(item["probe"])
        probe_found = probe in observed
        passed = recall >= policy[lang] and probe_found
        result = {
            "lang": lang, "normalized_char_recall": round(recall, 4),
            "threshold": policy[lang], "probe_found": probe_found,
            "characters_expected": len(source),
            "characters_observed": len(observed),
            "passed": passed, "tier": "searchable-image-only",
        }
        results.append(result)
        if not passed:
            errors.append(f"{lang}: recall={recall:.3f}, probe={probe_found}")
    (root / "ocr-quality.json").write_text(
        json.dumps(results, indent=2, ensure_ascii=False) + "\n",
        encoding="utf-8",
    )
    print(json.dumps(results, ensure_ascii=False))
    if errors:
        raise SystemExit("Synthetic OCR threshold failed: " + "; ".join(errors))


if __name__ == "__main__":
    main()
