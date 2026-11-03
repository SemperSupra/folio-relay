#!/usr/bin/env python3
"""Source-only Apple URF preview admission falsification, synthetic files only.

No installed services, real documents, NAS, metadata writes or credentials.
Tests the existing libcups grayscale one-page helper's fail-closed behavior;
success here does NOT admit other profiles, colorspaces, formats or a
production preview worker.
"""
import hashlib
import pathlib
import subprocess
import sys
import tempfile

def sha(path: pathlib.Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()

def run(*args: object) -> subprocess.CompletedProcess[str]:
    return subprocess.run([str(x) for x in args], capture_output=True, text=True)

def must_reject(helper: pathlib.Path, source: pathlib.Path, dest: pathlib.Path, label: str):
    if dest.exists():
        raise RuntimeError("unsafe fixture reused destination: " + label)
    proc = run(helper, "decode", source, dest)
    if proc.returncode == 0 or dest.exists():
        raise RuntimeError("FAIL: invalid URF admitted " + label + ": " + proc.stderr)
    print("PASS: rejected", label, "exit", proc.returncode)

def main() -> None:
    if len(sys.argv) != 5:
        raise SystemExit("usage: probe.py EXISTING_BRIDGE TWO_PAGE_GEN SOURCE_PGM WORKDIR")
    bridge, multipage, original, workdir = (pathlib.Path(a).resolve() for a in sys.argv[1:])
    workdir.mkdir(parents=True, exist_ok=True)
    source_digest = sha(original)
    valid = workdir / "valid.urf"
    proc = run(bridge, "encode", original, valid)
    if proc.returncode:
        raise RuntimeError("valid upstream URF encode failed: " + proc.stderr)
    data = valid.read_bytes()
    if len(data) < 256 or not data.startswith(b"UNIRAST"):
        raise RuntimeError("not genuine Apple raster output")
    print("PASS: upstream libcups produced Apple URF", len(data), "bytes")
    decoded = workdir / "decoded.pgm"
    proc = run(bridge, "decode", valid, decoded)
    if proc.returncode or decoded.read_bytes() != original.read_bytes():
        raise RuntimeError("one-page grayscale byte identity failed")
    print("PASS: exact source PGM identity")

    with tempfile.TemporaryDirectory(prefix="urf-negative-") as d:
        tmp = pathlib.Path(d)
        offsets = sorted({
            0, 1, 6, 7, 8, 20, 100,
            len(data) // 4, len(data) // 2, len(data) - 1
        })
        for offset in offsets:
            if offset >= len(data):
                continue
            inp = tmp / ("truncate-" + str(offset) + ".urf")
            inp.write_bytes(data[:offset])
            must_reject(bridge, inp, tmp / ("output-" + str(offset) + ".pgm"),
                        "truncated-" + str(offset))
        for where in (0, 6):
            corrupt = bytearray(data)
            corrupt[where] = ord("X")
            inp = tmp / ("corrupt-header-" + str(where) + ".urf")
            inp.write_bytes(corrupt)
            must_reject(bridge, inp, tmp / ("corrupt-" + str(where) + ".pgm"),
                        "invalid-magic-" + str(where))

        other = tmp / "two-page.urf"
        proc = run(multipage, valid, other)
        if proc.returncode or not other.is_file():
            raise RuntimeError("cannot generate genuine two-page URF negative: " + proc.stderr)
        must_reject(bridge, other, tmp / "multipage-output.pgm",
                    "valid-two-page-not-yet-supported")
        badpgm = tmp / "oversize.pgm"
        badpgm.write_bytes(b"P5\n5000 5000\n255\n")
        encoded = tmp / "oversize.urf"
        proc = run(bridge, "encode", badpgm, encoded)
        if proc.returncode == 0 or encoded.exists():
            raise RuntimeError("oversized PGM accepted for preview source")
        print("PASS: oversized PGM rejected")
        badpgm = tmp / "trailing.pgm"
        badpgm.write_bytes(original.read_bytes() + b"EXTRA")
        encoded = tmp / "trailing.urf"
        proc = run(bridge, "encode", badpgm, encoded)
        if proc.returncode == 0 or encoded.exists():
            raise RuntimeError("unexpected source trailing bytes accepted")
        print("PASS: trailing PGM bytes rejected")

    # RED: decoding must neither truncate a pre-existing output nor follow a
    # symlink into a canonical artifact. A failure must not unlink either.
    with tempfile.TemporaryDirectory(prefix="urf-output-negative-") as d:
        tmp = pathlib.Path(d)
        victim = tmp / "canonical-sentinel.txt"
        victim.write_bytes(b"DO-NOT-MODIFY-CANONICAL")
        link = tmp / "preview-link.pgm"
        link.symlink_to(victim)
        proc = run(bridge, "decode", valid, link)
        if proc.returncode == 0 or not link.is_symlink() or victim.read_bytes() != b"DO-NOT-MODIFY-CANONICAL":
            raise RuntimeError("RED: URF decoder followed or removed existing symlink output")
        print("PASS: existing symlink destination rejected without mutation")
        existing = tmp / "existing-output.pgm"
        existing.write_bytes(b"EXISTING-DATA-RETAINED")
        proc = run(bridge, "decode", valid, existing)
        if proc.returncode == 0 or existing.read_bytes() != b"EXISTING-DATA-RETAINED":
            raise RuntimeError("RED: URF decoder overwrote existing destination")
        print("PASS: existing regular output rejected without mutation")

    if sha(original) != source_digest:
        raise RuntimeError("canonical fixture was modified")
    print("PASS: original fixture unchanged sha256=" + source_digest)
    print("NOT PROVEN: robust production URF->PDF conversion, multicolor, multipage, quarantine and deployment")

if __name__ == "__main__":
    main()
