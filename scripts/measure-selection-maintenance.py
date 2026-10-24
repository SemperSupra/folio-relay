#!/usr/bin/env python3
"""Measure deterministic FolioRelay integration/maintenance surface per substrate."""
from __future__ import annotations

import json
import pathlib
import re
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parents[1]

CANDIDATES = {
    "cups": {
        "glue": [
            "experiments/print-substrate/cups-backend.sh",
            "deploy/engineering-cups/start-cups.sh",
        ],
        "config": [
            "deploy/engineering-cups/Dockerfile",
            "deploy/engineering-cups/compose.yaml",
            "deploy/engineering-cups/cups-files.conf",
            "deploy/engineering-cups/cupsd.conf",
            "deploy/engineering-cups/printers.conf",
            "experiments/print-substrate/cups-foliorelay.ppd",
        ],
        "patch_scan": [
            "deploy/engineering-cups/Dockerfile",
            ".github/workflows/macos-native-pappl-client.yml",
            ".github/workflows/windows-wsl-candidate-host.yml",
        ],
        "keywords": ("cups",),
    },
    "pappl": {
        "glue": [
            "experiments/print-substrate/pappl-ingress.c",
            "deploy/engineering-pappl/start-pappl.sh",
        ],
        "config": [
            "deploy/engineering-pappl/Dockerfile",
            "deploy/engineering-pappl/compose.yaml",
        ],
        "patch_scan": [
            "deploy/engineering-pappl/Dockerfile",
            ".github/workflows/macos-native-pappl-client.yml",
            ".github/workflows/windows-wsl-candidate-host.yml",
        ],
        "keywords": ("pappl",),
    },
}

PATCH_COMMAND_RE = re.compile(r"\b(?:git\s+apply|patch\s+-p\d*|quilt\s+push)\b", re.I)


def physical_and_effective(path: pathlib.Path) -> tuple[int, int]:
    text = path.read_text(errors="replace")
    lines = text.splitlines()
    physical = len(lines)

    # Count nonblank, non-comment-only lines. For C glue, suppress lines wholly
    # inside block comments as well; this is a characterization metric, not SLOC.
    effective = 0
    in_block = False
    for raw in lines:
        line = raw.strip()
        if not line:
            continue
        if in_block:
            if "*/" in line:
                in_block = False
                line = line.split("*/", 1)[1].strip()
                if not line:
                    continue
            else:
                continue
        if line.startswith("/*"):
            if "*/" in line:
                line = line.split("*/", 1)[1].strip()
                if not line:
                    continue
            else:
                in_block = True
                continue
        if line.startswith("//") or line.startswith("#"):
            continue
        effective += 1
    return physical, effective


def measure_files(paths: list[str]) -> dict:
    details = {}
    physical = effective = 0
    for rel in paths:
        path = ROOT / rel
        if not path.is_file():
            raise SystemExit(f"required measurement file missing: {rel}")
        p, e = physical_and_effective(path)
        details[rel] = {"physical_lines": p, "effective_lines": e}
        physical += p
        effective += e
    return {
        "physical_lines": physical,
        "effective_lines": effective,
        "files": details,
    }


def tracked_files() -> list[str]:
    result = subprocess.run(
        ["git", "ls-files"], cwd=ROOT, check=True, text=True, capture_output=True
    )
    return [line for line in result.stdout.splitlines() if line]


def patch_evidence(candidate: str, cfg: dict) -> dict:
    patch_files = []
    for rel in tracked_files():
        lower = rel.lower()
        if not lower.endswith((".patch", ".diff")):
            continue
        if any(keyword in lower for keyword in cfg["keywords"]) or "/patches/" in f"/{lower}":
            patch_files.append(rel)

    patch_commands = []
    for rel in cfg["patch_scan"]:
        path = ROOT / rel
        if not path.is_file():
            continue
        for number, line in enumerate(path.read_text(errors="replace").splitlines(), 1):
            if PATCH_COMMAND_RE.search(line):
                patch_commands.append({"file": rel, "line": number, "text": line.strip()})

    return {
        "carried_patch_file_count": len(patch_files),
        "carried_patch_files": sorted(patch_files),
        "source_patch_command_count": len(patch_commands),
        "source_patch_commands": patch_commands,
        "carried_patch_count": len(patch_files) + len(patch_commands),
    }


def main() -> None:
    if len(sys.argv) != 3 or sys.argv[1] not in CANDIDATES:
        raise SystemExit(f"usage: {sys.argv[0]} <cups|pappl> OUTPUT.json")

    candidate = sys.argv[1]
    output = pathlib.Path(sys.argv[2])
    cfg = CANDIDATES[candidate]

    result = {
        "candidate": candidate,
        "glue": measure_files(cfg["glue"]),
        "configuration": measure_files(cfg["config"]),
        "patches": patch_evidence(candidate, cfg),
    }
    result["folio_relay_glue_loc"] = result["glue"]["effective_lines"]
    result["candidate_configuration_loc"] = result["configuration"]["effective_lines"]
    result["carried_patch_count"] = result["patches"]["carried_patch_count"]

    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(result, indent=2, sort_keys=True) + "\n")
    print(json.dumps(result, sort_keys=True))


if __name__ == "__main__":
    main()
