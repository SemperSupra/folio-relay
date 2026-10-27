#!/usr/bin/env python3
"""Disposable generated fixture only. Not a FolioRelay deployed runtime."""
import sys
from reportlab.lib.pagesizes import A4
from reportlab.pdfgen import canvas

if len(sys.argv) != 2:
    raise SystemExit("usage: make_synthetic_pdf.py OUTPUT")
pdf = canvas.Canvas(sys.argv[1], pagesize=A4, pageCompression=0)
pdf.setTitle("Native source fixture")
pdf.setAuthor("Synthetic Test Harness")
pdf.setFont("Helvetica-Bold", 22)
pdf.drawString(42, 735, "FolioRelay SYNTHETIC receipt")
pdf.setFont("Helvetica", 16)
pdf.drawString(42, 680, "This document contains editable source text.")
pdf.drawString(42, 646, "Only fake text is included in this qualification.")
pdf.drawString(42, 612, "OCR must recover FolioRelay and SYNTHETIC.")
pdf.line(42, 594, 510, 594)
pdf.showPage()
pdf.save()
