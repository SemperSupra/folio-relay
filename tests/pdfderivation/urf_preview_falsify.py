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
    return subprocess.run([str(x) for x in args], capture_output=True, text=True, timeout=20)

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

    # Independent encode-path RED: failed O_EXCL must never unlink an
    # existing output. The previous cleanup unlinked(target) unconditionally.
    with tempfile.TemporaryDirectory(prefix="urf-encode-ownership-") as d:
        tmp = pathlib.Path(d)
        victim = tmp / "canonical-sentinel.txt"
        victim.write_bytes(b"DO-NOT-DELETE-CANONICAL")
        existing = tmp / "pre-existing.urf"
        existing.write_bytes(b"PREEXISTING-URF-BYTES")
        proc = run(bridge, "encode", original, existing)
        if proc.returncode == 0 or not existing.is_file() or existing.read_bytes() != b"PREEXISTING-URF-BYTES":
            raise RuntimeError("RED: failed encoder removed or overwrote pre-existing output")
        print("PASS: pre-existing encoder output preserved on exclusive-open failure")
        link = tmp / "pre-existing-link.urf"
        link.symlink_to(victim)
        proc = run(bridge, "encode", original, link)
        if proc.returncode == 0 or not link.is_symlink() or victim.read_bytes() != b"DO-NOT-DELETE-CANONICAL":
            raise RuntimeError("RED: failed encoder unlinked symlink or touched canonical victim")
        print("PASS: pre-existing encoder symlink and its target preserved")

    # urf-source-open-negatives: reject nonregular and symlink sources, and
    # do not wait on an untrusted FIFO before file-type inspection.
    with tempfile.TemporaryDirectory(prefix="urf-source-open-negatives-") as d:
        tmp = pathlib.Path(d)
        urf_link = tmp / "link.urf"
        urf_link.symlink_to(valid)
        must_reject(bridge, urf_link, tmp / "symlink-decoded.pgm", "source-URF-symlink")
        pgm_link = tmp / "link.pgm"
        pgm_link.symlink_to(original)
        encoded_link = tmp / "from-symlink.urf"
        proc = run(bridge, "encode", pgm_link, encoded_link)
        if proc.returncode == 0 or encoded_link.exists():
            raise RuntimeError("RED: symlink PGM source admitted by encoder")
        print("PASS: source PGM symlink rejected")
        fifo = tmp / "no-writer.urf"
        import os
        os.mkfifo(fifo, 0o600)
        must_reject(bridge, fifo, tmp / "fifo-decoded.pgm", "source-URF-FIFO-no-writer")
        encoded_fifo = tmp / "from-fifo.urf"
        proc = run(bridge, "encode", fifo, encoded_fifo)
        if proc.returncode == 0 or encoded_fifo.exists():
            raise RuntimeError("RED: FIFO PGM source admitted or output left behind")
        print("PASS: source PGM FIFO rejected without waiting for a writer")
        directory = tmp / "directory-source"
        directory.mkdir()
        must_reject(bridge, directory, tmp / "dir-decoded.pgm", "source-URF-directory")
        encoded_directory = tmp / "from-directory.urf"
        proc = run(bridge, "encode", directory, encoded_directory)
        if proc.returncode == 0 or encoded_directory.exists():
            raise RuntimeError("RED: directory PGM source admitted by encoder")
        print("PASS: directory sources rejected")

    if sha(original) != source_digest:
        raise RuntimeError("canonical fixture was modified")
    print("PASS: original fixture unchanged sha256=" + source_digest)
    print("NOT PROVEN: robust production URF->PDF conversion, multicolor, multipage, quarantine and deployment")

if __name__ == "__main__":
    main()
