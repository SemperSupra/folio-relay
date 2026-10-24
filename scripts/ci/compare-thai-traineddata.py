#!/usr/bin/env python3
"""Compare baseline Thai Tesseract traineddata to verified upstream best/fast.

Only generated Thai/mixed-language PNGs; immutable corpus and exact Git blob
pinned traineddata are required. Reports ordered Thai codepoint CER and time.
Does NOT write Paperless metadata, OCR source, or native filing policy.
"""
import json
import os
import pathlib
import runpy
import subprocess
import sys
import time

def main():
    root=pathlib.Path(sys.argv[1])
    src=runpy.run_path("scripts/ci/qualify-thai-tesseract-sweep.py")
    chars=src["chars"]
    edit=src["edit_distance"]
    candidates={
        "ubuntu_packaged":pathlib.Path(sys.argv[2]),
        "tessdata_best":pathlib.Path(sys.argv[3]),
        "tessdata_fast":pathlib.Path(sys.argv[4]),
    }
    cases=[x for x in json.loads((root/"classification-manifest.json").read_text("utf-8"))
           if x["language"]=="th"]
    mixed=next(x for x in json.loads((root/"manifest.json").read_text("utf-8"))
               if x["id"]=="mixed")
    cases.append(dict(mixed,negative=False,kind="mixed"))
    if len(cases)!=9:raise ValueError("incomplete 6+2+1 held-out cases")
    reports=[]
    for name,path in candidates.items():
        if not (path/"tha.traineddata").exists():
            raise RuntimeError("missing Thai traineddata for "+name)
        rows=[]
        for item in cases:
            t0=time.perf_counter()
            result=subprocess.run(
                ["tesseract",str(root/item["file"]),"stdout",
                 "--tessdata-dir",str(path),
                 "-l","eng+deu+tha","--psm","3","--oem","1"],
                capture_output=True,text=True,timeout=90,check=True
            )
            elapsed=round((time.perf_counter()-t0)*1000)
            expected=chars(" ".join(item["lines"]))
            actual=chars(result.stdout)
            d=edit(expected,actual)
            rows.append({
                "id":item["id"],
                "negative":bool(item.get("negative",False)),
                "reference_thai_chars":len(expected),
                "edits":d,
                "cer":round(d/max(1,len(expected)),4),
                "latency_ms":elapsed,
            })
        denom=sum(x["reference_thai_chars"] for x in rows)
        errors=sum(x["edits"] for x in rows)
        reports.append({
            "traineddata":name,"cases":len(rows),
            "thai_codepoint_cer":round(errors/denom,4),
            "total_ocr_milliseconds":sum(x["latency_ms"] for x in rows),
            "per_case":rows
        })
    baseline=reports[0]["thai_codepoint_cer"]
    print("FOLIO_THAI_TRAINEDDATA_AB="+json.dumps({
        "scope":"direct Tesseract only, 9 generated Thai/mixed PNGs",
        "profile":"eng+deu+tha, PSM3, OEM1",
        "baseline_cer":baseline,
        "results":reports,
        "writes_to_dms":False,
        "auto_apply":False
    },ensure_ascii=False,separators=(",",":")))
    return 0

if __name__=="__main__":
    sys.exit(main())
