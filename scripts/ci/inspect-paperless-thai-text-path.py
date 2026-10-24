#!/usr/bin/env python3
"""Locate Thai OCR corruption across four real, synthetic-only text channels.

This is a diagnostic, not a content rewriter or a classifier. It exercises:
raw Tesseract stdout -> OCRmyPDF sidecar -> pdftotext(PDF) ->
Paperless v3.1.3 persisted Document.content for the same raster.
"""
import hashlib
import json
import os
import pathlib
import re
import runpy
import subprocess
import sys
import time
import urllib.request
import uuid

def run(args, timeout=120):
    p=subprocess.run(args,text=True,stdout=subprocess.PIPE,
                     stderr=subprocess.PIPE,timeout=timeout,check=True)
    return p.stdout

def api(base,token,method,endpoint,body=None,ctype=None):
    req=urllib.request.Request(base+endpoint,data=body,method=method)
    req.add_header("Authorization","Token "+token)
    req.add_header("Accept","application/json; version=10")
    if ctype:req.add_header("Content-Type",ctype)
    with urllib.request.urlopen(req,timeout=45) as response:
        return json.load(response)

def upload(base,token,image,filename):
    boundary="foliorelay-"+uuid.uuid4().hex
    payload=(
        ("--"+boundary+"\r\nContent-Disposition: form-data; "
         'name="document"; filename="'+filename+'"\r\n'
         "Content-Type: image/png\r\n\r\n").encode("utf-8")
         +image+b"\r\n--"+boundary.encode()+b"--\r\n"
    )
    return api(base,token,"POST","/api/documents/post_document/",payload,
               "multipart/form-data; boundary="+boundary)

def await_archive(base,token,task_id):
    limit=time.monotonic()+220
    while time.monotonic()<limit:
        payload=api(base,token,"GET","/api/tasks/?task_id="+task_id)
        results=payload.get("results",[])
        if len(results)!=1:
            time.sleep(2);continue
        r=results[0]
        if r.get("status")=="success":
            doc_id=r.get("result_data",{}).get("document_id",0)
            if not isinstance(doc_id,int) or doc_id<1:
                raise RuntimeError("Paperless archived task without id")
            return doc_id
        if r.get("status") in ("failure","revoked"):
            raise RuntimeError("Paperless failed synthetic task")
        time.sleep(2)
    raise RuntimeError("Paperless did not complete synthetic OCR")

def compare(reference, text, chars, distance):
    ref=chars(reference)
    got=chars(text)
    match=len(re.findall(r"[\u0e00-\u0e7f][ \t]+[\u0e00-\u0e7f]",text))
    return {"thai_reference_chars":len(ref),"thai_observed_chars":len(got),
            "thai_edit_distance":distance(ref,got),
            "thai_cer":round(distance(ref,got)/max(1,len(ref)),4),
            "intra_thai_whitespace_pairs":match}

def main():
    root=pathlib.Path(sys.argv[1])
    src=runpy.run_path("scripts/ci/qualify-thai-tesseract-sweep.py")
    chars=src["chars"]; distance=src["edit_distance"]
    cases=json.loads((root/"classification-manifest.json").read_text("utf-8"))
    mixed=json.loads((root/"manifest.json").read_text("utf-8"))
    targets=[next(x for x in cases if x["id"]==k)
             for k in ("invoice-th","appointment-th")]
    targets.append(next(x for x in mixed if x["id"]=="mixed"))
    base=os.environ["FOLIO_FIXTURE_PAPERLESS_URL"]
    token=os.environ["FOLIO_FIXTURE_PAPERLESS_TOKEN"]
    if not token or not base.startswith("http://127.0.0.1:"):
        raise RuntimeError("Only explicitly admitted disposable local Paperless fixture")
    measurements=[]
    for i,item in enumerate(targets):
        filename=item["id"]+".png"
        local=root/filename
        if not local.is_file():raise RuntimeError("missing generated synthetic PNG")
        path="/tmp/folio-thai-"+str(i)
        run(["docker","cp",str(local),"folio_ci_paperless:"+path+".png"])
        # Direct OCR's TXT differs from sidecar/PDF serialization for
        # non-space-delimited scripts; do not assume origin of corruption.
        raw=run(["docker","exec","folio_ci_paperless","tesseract",
                 path+".png","stdout","-l","eng+deu+tha","--psm","3"])
        run(["docker","exec","folio_ci_paperless","ocrmypdf",
             "--image-dpi","300","-l","eng+deu+tha","--sidecar",path+".txt",
             "--output-type","pdf",path+".png",path+".pdf"],timeout=180)
        sidecar=run(["docker","exec","folio_ci_paperless","cat",path+".txt"])
        pdftext=run(["docker","exec","folio_ci_paperless","pdftotext",
                    "-q","-layout","-enc","UTF-8",path+".pdf","-"])
        tid=upload(base,token,local.read_bytes(),filename)
        if not isinstance(tid,str) or len(tid)<30:
            raise RuntimeError("Malformed Paperless upload task response")
        doc_id=await_archive(base,token,tid)
        record=api(base,token,"GET",f"/api/documents/{doc_id}/")
        persisted=record.get("content")
        if not isinstance(persisted,str):
            raise RuntimeError("Missing persisted OCR content")
        reference=" ".join(item["lines"])
        measurements.append({
            "id":item["id"],
            "image_sha256":hashlib.sha256(local.read_bytes()).hexdigest(),
            "direct_tesseract":compare(reference,raw,chars,distance),
            "ocrmypdf_sidecar":compare(reference,sidecar,chars,distance),
            "pdf_pdftotext":compare(reference,pdftext,chars,distance),
            "paperless_document_content":compare(reference,persisted,chars,distance)
        })
    print("FOLIO_THAI_OCR_TEXT_PATH="+json.dumps({
        "scope":"3 generated PNGs only; 3.1.3 container",
        "ocr_languages":"eng+deu+tha",
        "paperless_auto_apply":False,
        "channels":measurements
    },ensure_ascii=False,separators=(",",":")))
    return 0

if __name__=="__main__":
    sys.exit(main())
