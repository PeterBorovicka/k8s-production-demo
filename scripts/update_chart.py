#!/usr/bin/env python3
"""Update immutable image coordinates and bump the Helm chart patch version."""

from __future__ import annotations

import argparse
import re
from pathlib import Path


def update_values(path: Path, frontend_repository: str, backend_repository: str, tag: str) -> None:
    replacements = {
        "frontend": {"repository": frontend_repository, "tag": tag},
        "backend": {"repository": backend_repository, "tag": tag},
    }
    found: set[tuple[str, str]] = set()
    section = None
    image = None
    output: list[str] = []

    for line in path.read_text(encoding="utf-8").splitlines(keepends=True):
        if re.match(r"^images:\s*$", line.rstrip("\n")):
            section = "images"
            image = None
        elif line and not line.startswith((" ", "\t", "\n", "\r")):
            section = None
            image = None
        elif section == "images":
            child = re.match(r"^  (frontend|backend):\s*$", line.rstrip("\n"))
            if child:
                image = child.group(1)
            elif image:
                field = re.match(r"^(    )(repository|tag):.*$", line.rstrip("\n"))
                if field:
                    key = field.group(2)
                    value = replacements[image][key]
                    quoted = f'"{value}"' if key == "tag" else value
                    line = f"{field.group(1)}{key}: {quoted}\n"
                    found.add((image, key))
        output.append(line)

    expected = {(image_name, key) for image_name in replacements for key in ("repository", "tag")}
    if found != expected:
        missing = ", ".join(f"{image}.{key}" for image, key in sorted(expected - found))
        raise ValueError(f"missing image fields in {path}: {missing}")
    path.write_text("".join(output), encoding="utf-8")


def update_chart(path: Path, app_version: str) -> str:
    text = path.read_text(encoding="utf-8")
    match = re.search(r"(?m)^version: (\d+)\.(\d+)\.(\d+)$", text)
    if not match:
        raise ValueError(f"semantic chart version not found in {path}")
    major, minor, patch = (int(value) for value in match.groups())
    new_version = f"{major}.{minor}.{patch + 1}"
    text = text[: match.start()] + f"version: {new_version}" + text[match.end() :]
    text, count = re.subn(r'(?m)^appVersion: .*$', f'appVersion: "{app_version}"', text)
    if count != 1:
        raise ValueError(f"expected exactly one appVersion in {path}, got {count}")
    path.write_text(text, encoding="utf-8")
    return new_version


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--values", type=Path, default=Path("helm/webapp/values.yaml"))
    parser.add_argument("--chart", type=Path, default=Path("helm/webapp/Chart.yaml"))
    parser.add_argument("--frontend-repository", required=True)
    parser.add_argument("--backend-repository", required=True)
    parser.add_argument("--tag", required=True)
    args = parser.parse_args()

    update_values(args.values, args.frontend_repository, args.backend_repository, args.tag)
    version = update_chart(args.chart, args.tag)
    print(version)


if __name__ == "__main__":
    main()

