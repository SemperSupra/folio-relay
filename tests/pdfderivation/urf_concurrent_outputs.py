#!/usr/bin/env python3
"""Disposable, synthetic-only competing libcups URF preview-output experiment.

Two independent decoder processes race to claim the same new preview output.
One must own it; the other must fail, without modifying source, replacement,
or a pre-existing sentinel. No installed worker, real records, or TrueNAS.
This does not qualify a hostile writer that can rename/unlink an owned path.
"""
from __future__ import annotations
import hashlib
import pathlib
import subprocess
import sys
import tempfile


def digest(path: pathlib.Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main() -> None:
    if len(sys.argv) != 4:
        raise SystemExit("usage: urf_concurrent_outputs.py BRIDGE VALID.urf ORIGINAL.pgm")
    bridge, valid, original = (pathlib.Path(p).resolve() for p in sys.argv[1:])
    before_urf, before_pgm = digest(valid), digest(original)
    with tempfile.TemporaryDirectory(prefix="urf-preview-competition-") as dir_name:
        directory = pathlib.Path(dir_name)
        output = directory / "unique-preview.pgm"
        procs = [
            subprocess.Popen(
                [str(bridge), "decode", str(valid), str(output)],
                stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
            for _ in range(2)
        ]
        try:
            results = [p.communicate(timeout=20) for p in procs]
        except subprocess.TimeoutExpired as e:
            for p in procs:
                p.kill()
                p.communicate()
            raise RuntimeError("FAIL: two competing decoders exceeded source-only process budget") from e
        statuses = [p.returncode for p in procs]
        if sorted(s == 0 for s in statuses) != [False, True]:
            raise RuntimeError(f"FAIL: concurrent output admission ambiguous {statuses!r}; stderr={[r[1] for r in results]!r}")
        if not output.is_file() or output.is_symlink() or output.read_bytes() != original.read_bytes():
            raise RuntimeError("FAIL: winner output is not the immutable expected PGM")
        if digest(valid) != before_urf or digest(original) != before_pgm:
            raise RuntimeError("FAIL: source URF/PGM changed by competing writers")
        if sorted(x.name for x in directory.iterdir()) != ["unique-preview.pgm"]:
            raise RuntimeError("FAIL: unexpected competing-writer output or scratch residue")
        print("PASS: exactly one exclusive owner among two source-only decoder processes")
        print("PASS: admitted output equals original synthetic pixel PGM")
        print("PASS: URF and PGM originals unchanged; source SHA-256", before_urf, before_pgm)
    print("NOT PROVEN: hostile rename/unlink, adverse cancellation during partial output, filesystem isolation or production safe URF profile")


if __name__ == "__main__":
    main()
