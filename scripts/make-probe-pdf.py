#!/usr/bin/env python3
from pathlib import Path
import sys

if len(sys.argv) != 2:
    raise SystemExit("usage: make-probe-pdf.py OUTPUT.pdf")

out = Path(sys.argv[1])
text = b"BT /F1 18 Tf 72 720 Td (FolioRelay native driverless client probe) Tj ET\n"
objects = [
    b"<< /Type /Catalog /Pages 2 0 R >>",
    b"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
    b"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
    b"<< /Length %d >>\nstream\n" % len(text) + text + b"endstream",
    b"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
]

buf = bytearray(b"%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
offsets = [0]
for index, obj in enumerate(objects, 1):
    offsets.append(len(buf))
    buf.extend(f"{index} 0 obj\n".encode())
    buf.extend(obj)
    buf.extend(b"\nendobj\n")

xref = len(buf)
buf.extend(f"xref\n0 {len(objects) + 1}\n".encode())
buf.extend(b"0000000000 65535 f \n")
for offset in offsets[1:]:
    buf.extend(f"{offset:010d} 00000 n \n".encode())
buf.extend(
    (
        f"trailer\n<< /Size {len(objects) + 1} /Root 1 0 R >>\n"
        f"startxref\n{xref}\n%%EOF\n"
    ).encode()
)

out.write_bytes(buf)
