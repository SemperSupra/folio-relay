#!/usr/bin/env python3
from __future__ import annotations

import argparse
import pathlib
import re
import subprocess
import sys
import time
import xml.etree.ElementTree as ET

BOUNDS = re.compile(r"\[(\d+),(\d+)\]\[(\d+),(\d+)\]")


def adb(*args: str, check: bool = True) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        ["adb", *args],
        check=check,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
    )


def snapshot(out: pathlib.Path, label: str) -> ET.Element:
    out.mkdir(parents=True, exist_ok=True)
    adb("shell", "uiautomator", "dump", "/sdcard/foliorelay-window.xml", check=False)
    result = adb("exec-out", "cat", "/sdcard/foliorelay-window.xml", check=False)
    (out / f"{label}.xml").write_text(result.stdout)
    with (out / f"{label}.png").open("wb") as handle:
        raw = subprocess.run(
            ["adb", "exec-out", "screencap", "-p"],
            check=False,
            stdout=subprocess.PIPE,
        ).stdout
        handle.write(raw)
    try:
        return ET.fromstring(result.stdout)
    except ET.ParseError as error:
        raise SystemExit(f"unable to parse UI hierarchy for {label}: {error}")


def center(node: ET.Element) -> tuple[int, int]:
    match = BOUNDS.fullmatch(node.attrib.get("bounds", ""))
    if not match:
        raise ValueError("node has no usable bounds")
    x1, y1, x2, y2 = map(int, match.groups())
    return (x1 + x2) // 2, (y1 + y2) // 2


def nodes(root: ET.Element):
    yield from root.iter("node")


def matches(node: ET.Element, kind: str, value: str) -> bool:
    if kind == "text":
        return node.attrib.get("text", "") == value
    if kind == "contains":
        return value.lower() in node.attrib.get("text", "").lower()
    if kind == "res":
        return node.attrib.get("resource-id", "") == value
    if kind == "desc":
        return value.lower() in node.attrib.get("content-desc", "").lower()
    if kind == "edit":
        return node.attrib.get("class", "") == "android.widget.EditText"
    raise ValueError(kind)


def find(
    out: pathlib.Path,
    kind: str,
    value: str,
    timeout: float,
    *,
    enabled_only: bool = False,
) -> ET.Element | None:
    deadline = time.time() + timeout
    attempt = 0
    while time.time() < deadline:
        attempt += 1
        root = snapshot(out, f"wait-{kind}-{attempt:02d}")
        for node in nodes(root):
            if not matches(node, kind, value):
                continue
            if enabled_only and node.attrib.get("enabled", "true").lower() != "true":
                continue
            return node
        time.sleep(1)
    return None


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--evidence", required=True)
    parser.add_argument("command", choices=["wait", "tap", "clear", "snapshot"])
    parser.add_argument("kind", nargs="?", choices=["text", "contains", "res", "desc", "edit"])
    parser.add_argument("value", nargs="?", default="")
    parser.add_argument("--timeout", type=float, default=20)
    parser.add_argument("--label", default="snapshot")
    args = parser.parse_args()
    out = pathlib.Path(args.evidence)

    if args.command == "snapshot":
        snapshot(out, args.label)
        return

    if args.kind is None:
        raise SystemExit("kind is required")

    node = find(
        out,
        args.kind,
        args.value,
        args.timeout,
        enabled_only=(args.command in {"tap", "clear"}),
    )
    if node is None:
        raise SystemExit(f"UI node not found: {args.kind}={args.value!r}")

    if args.command == "tap":
        x, y = center(node)
        adb("shell", "input", "tap", str(x), str(y))
        print(f"tapped {args.kind}={args.value!r} at {x},{y}")
    elif args.command == "clear":
        if node.attrib.get("class", "") != "android.widget.EditText":
            raise SystemExit(
                f"clear target is not EditText: {args.kind}={args.value!r}"
            )
        current = node.attrib.get("text", "")
        if node.attrib.get("focused", "false").lower() != "true":
            x, y = center(node)
            adb("shell", "input", "tap", str(x), str(y))
            time.sleep(0.25)
        # Some Android/BIPS EditTexts keep the cursor at the beginning even
        # after KEYCODE_MOVE_END, making backspace a no-op. Try the ordinary
        # end/backspace path first, then fall back to home/forward-delete based
        # on the actual remaining text observed from the UI hierarchy.
        adb("shell", "input", "keyevent", "KEYCODE_MOVE_END")
        for _ in current:
            adb("shell", "input", "keyevent", "KEYCODE_DEL")

        root = snapshot(out, "clear-after-backspace")
        edits = [
            item for item in nodes(root)
            if item.attrib.get("class", "") == "android.widget.EditText"
            and (
                args.kind != "res"
                or item.attrib.get("resource-id", "") == args.value
            )
        ]
        remaining = edits[0].attrib.get("text", "") if edits else current
        if remaining:
            adb("shell", "input", "keyevent", "KEYCODE_MOVE_HOME")
            for _ in remaining:
                adb("shell", "input", "keyevent", "KEYCODE_FORWARD_DEL")

        deadline = time.time() + args.timeout
        attempt = 0
        while time.time() < deadline:
            attempt += 1
            root = snapshot(out, f"clear-edit-{attempt:02d}")
            edits = [
                item for item in nodes(root)
                if item.attrib.get("class", "") == "android.widget.EditText"
                and (
                    args.kind != "res"
                    or item.attrib.get("resource-id", "") == args.value
                )
            ]
            if edits and edits[0].attrib.get("text", "") == "":
                print(f"cleared edit text ({len(current)} characters)")
                return
            time.sleep(0.5)
        raise SystemExit(
            f"EditText did not clear; original value length={len(current)}"
        )
    else:
        print(f"found {args.kind}={args.value!r}")


if __name__ == "__main__":
    main()
