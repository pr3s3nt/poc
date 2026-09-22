#!/usr/bin/env python3
"""Validate the repository's AI-facing documentation contract."""

from __future__ import annotations

import os
import re
import shutil
import subprocess
import sys
from pathlib import Path
from urllib.parse import unquote


ROOT = Path(__file__).resolve().parents[1]
DOCS = ROOT / "orchestrator_docs"
ALLOWED_STATUS = {
    "draft",
    "current",
    "superseded",
    "deferred",
    "historical",
    "evidence",
}
LINK_RE = re.compile(r"(?<!!)\[[^\]]*\]\(([^)]+)\)")
USE_CASE_RE = re.compile(r"UC-\d{2}")
MILESTONE_DIR_RE = re.compile(r"M\d{2}-[a-z0-9-]+")
ITERATION_DIR_RE = re.compile(r"I\d{2}-\d{2}-[a-z0-9-]+")
REQUIRED_USE_CASE_FILES = (
    "README.md",
    "specification.md",
    "realization.md",
    "sequence.puml",
    "vopc.puml",
)
REQUIRED_ITERATION_FILES = ("README.md", "WORK_ITEMS.md")
RETIRED_ROOT_PATHS = (
    "AGENT.md",
    "deployment-delta-problem.md",
    "implementation",
    "final_idp",
    "humanitec-planner-challenge-v4",
)


def tracked_markdown() -> set[Path]:
    result = subprocess.run(
        ["git", "ls-files", "--", "*.md"],
        cwd=ROOT,
        check=True,
        capture_output=True,
        text=True,
    )
    return {ROOT / line for line in result.stdout.splitlines() if line}


def repository_files() -> set[Path]:
    tracked = subprocess.run(
        ["git", "ls-files", "-z"],
        cwd=ROOT,
        check=True,
        capture_output=True,
    )
    untracked = subprocess.run(
        ["git", "ls-files", "--others", "--exclude-standard", "-z"],
        cwd=ROOT,
        check=True,
        capture_output=True,
    )
    paths: set[Path] = set()
    for raw in (*tracked.stdout.split(b"\0"), *untracked.stdout.split(b"\0")):
        if not raw:
            continue
        path = ROOT / raw.decode("utf-8", errors="surrogateescape")
        if path.is_file():
            paths.add(path.resolve())
    return paths


def markdown_candidates() -> list[Path]:
    files = {path for path in tracked_markdown() if path.exists()}
    files.update(DOCS.rglob("*.md"))
    files.update((ROOT / "backend").rglob("*.md"))
    files.update(
        path
        for path in (
            ROOT / "README.md",
            ROOT / "AGENTS.md",
            ROOT / "plan.md",
            ROOT / "orchestrator_reference/README.md",
        )
        if path.exists()
    )
    return sorted(files)


def parse_front_matter(text: str) -> tuple[dict[str, str], str | None]:
    if not text.startswith("---\n"):
        return {}, "missing YAML front matter"
    end = text.find("\n---\n", 4)
    if end < 0:
        return {}, "front matter has no closing delimiter"

    values: dict[str, str] = {}
    for line in text[4:end].splitlines():
        if not line or line[0].isspace() or ":" not in line:
            continue
        key, value = line.split(":", 1)
        values[key.strip()] = value.strip().strip("\"'")

    missing = [
        key
        for key in ("id", "artifact", "status", "last_reviewed")
        if not values.get(key)
    ]
    if missing:
        return values, f"front matter is missing: {', '.join(missing)}"
    if values["status"] not in ALLOWED_STATUS:
        return values, f"invalid status: {values['status']}"
    return values, None


def normalized_link(raw: str) -> str:
    raw = raw.strip()
    if raw.startswith("<") and raw.endswith(">"):
        return raw[1:-1]
    return raw


def validate_use_case_packages(errors: list[str]) -> None:
    use_case_root = DOCS / "usecase"
    use_case_index = use_case_root / "README.md"
    docs_index = DOCS / "INDEX.md"
    if not use_case_index.is_file() or not docs_index.is_file():
        return

    use_case_index_text = use_case_index.read_text(encoding="utf-8")
    docs_index_text = docs_index.read_text(encoding="utf-8")
    for directory in sorted(path for path in use_case_root.glob("UC-*") if path.is_dir()):
        if not USE_CASE_RE.fullmatch(directory.name):
            errors.append(f"invalid use-case directory: {directory.relative_to(ROOT)}")
            continue
        for filename in REQUIRED_USE_CASE_FILES:
            path = directory / filename
            if not path.is_file():
                errors.append(f"incomplete use-case package; missing: {path.relative_to(ROOT)}")

        context_target = f"{directory.name}/README.md"
        if f"({context_target})" not in use_case_index_text:
            errors.append(
                f"orchestrator_docs/usecase/README.md does not register {context_target}"
            )
        docs_route = f"usecase/{directory.name}/README.md"
        if docs_route not in docs_index_text:
            errors.append(f"orchestrator_docs/INDEX.md does not route {docs_route}")


def validate_diagram_pairs(errors: list[str]) -> None:
    for source in sorted(DOCS.rglob("*.puml")):
        rendered = source.with_suffix(".png")
        if not rendered.is_file():
            errors.append(
                f"PlantUML source has no sibling PNG: {source.relative_to(ROOT)}"
            )


def validate_iteration_packages(errors: list[str]) -> None:
    iteration_root = DOCS / "iterations"
    iteration_index = iteration_root / "README.md"
    if not iteration_index.is_file():
        return

    index_text = iteration_index.read_text(encoding="utf-8")
    current_iterations: list[Path] = []
    current_milestones: list[Path] = []

    legacy_records = sorted(
        path for path in iteration_root.glob("*.md") if path.name != "README.md"
    )
    for path in legacy_records:
        errors.append(
            "iteration record must live in a milestone/iteration folder: "
            f"{path.relative_to(ROOT)}"
        )

    for milestone in sorted(path for path in iteration_root.glob("M*") if path.is_dir()):
        if not MILESTONE_DIR_RE.fullmatch(milestone.name):
            errors.append(f"invalid milestone directory: {milestone.relative_to(ROOT)}")
            continue

        milestone_readme = milestone / "README.md"
        if not milestone_readme.is_file():
            errors.append(
                f"milestone has no README: {milestone.relative_to(ROOT)}"
            )
            continue
        if f"({milestone.name}/README.md)" not in index_text:
            errors.append(
                f"iteration index does not register {milestone.name}/README.md"
            )

        milestone_text = milestone_readme.read_text(encoding="utf-8")
        milestone_metadata, milestone_error = parse_front_matter(milestone_text)
        if not milestone_error and milestone_metadata.get("status") == "current":
            current_milestones.append(milestone_readme)

        for iteration in sorted(path for path in milestone.glob("I*") if path.is_dir()):
            if not ITERATION_DIR_RE.fullmatch(iteration.name):
                errors.append(f"invalid iteration directory: {iteration.relative_to(ROOT)}")
                continue
            for filename in REQUIRED_ITERATION_FILES:
                path = iteration / filename
                if not path.is_file():
                    errors.append(
                        f"incomplete iteration package; missing: {path.relative_to(ROOT)}"
                    )

            iteration_readme = iteration / "README.md"
            if not iteration_readme.is_file():
                continue
            if f"({iteration.name}/README.md)" not in milestone_text:
                errors.append(
                    f"{milestone.relative_to(ROOT)}/README.md does not register "
                    f"{iteration.name}/README.md"
                )
            metadata, error = parse_front_matter(
                iteration_readme.read_text(encoding="utf-8")
            )
            if not error and metadata.get("status") == "current":
                current_iterations.append(iteration_readme)

    if len(current_milestones) != 1:
        errors.append(
            "exactly one milestone must be current; found "
            f"{len(current_milestones)}"
        )
    if len(current_iterations) != 1:
        errors.append(
            "exactly one iteration plan must be current; found "
            f"{len(current_iterations)}"
        )


def main() -> int:
    errors: list[str] = []
    ids: dict[str, Path] = {}
    repository_paths = repository_files()

    for relative in RETIRED_ROOT_PATHS:
        if (ROOT / relative).exists():
            errors.append(f"retired root path must not exist: {relative}")

    validate_use_case_packages(errors)
    validate_iteration_packages(errors)
    validate_diagram_pairs(errors)

    for path in markdown_candidates():
        rel = path.relative_to(ROOT)
        text = path.read_text(encoding="utf-8")

        if path.is_relative_to(DOCS) or text.startswith("---\n"):
            metadata, error = parse_front_matter(text)
            if error:
                errors.append(f"{rel}: {error}")
            else:
                artifact_id = metadata["id"]
                if artifact_id in ids:
                    errors.append(
                        f"{rel}: duplicate id {artifact_id} "
                        f"(also in {ids[artifact_id].relative_to(ROOT)})"
                    )
                else:
                    ids[artifact_id] = path

        for match in LINK_RE.finditer(text):
            raw = normalized_link(match.group(1))
            if raw.startswith(("http://", "https://", "mailto:", "#")):
                continue
            target_text = unquote(raw.split("#", 1)[0])
            if not target_text:
                continue
            line = text[: match.start()].count("\n") + 1
            if target_text.startswith("/"):
                errors.append(f"{rel}:{line}: absolute local link is not portable: {raw}")
                continue
            target = (path.parent / target_text).resolve()
            if not target.exists():
                errors.append(f"{rel}:{line}: missing link target: {raw}")
                continue
            if not target.is_relative_to(ROOT):
                errors.append(f"{rel}:{line}: link target escapes repository: {raw}")
                continue
            if target.is_file() and target not in repository_paths:
                errors.append(f"{rel}:{line}: link target is ignored or unversioned: {raw}")
                continue
            if target.is_dir() and not any(
                candidate.is_relative_to(target) for candidate in repository_paths
            ):
                errors.append(
                    f"{rel}:{line}: link target directory has no versioned files: {raw}"
                )

    require_plantuml = os.environ.get("REQUIRE_PLANTUML") == "1"
    plantuml = shutil.which("plantuml")
    if plantuml:
        diagrams = sorted(DOCS.rglob("*.puml"))
        result = subprocess.run(
            [plantuml, "-checkonly", *map(str, diagrams)],
            cwd=ROOT,
            check=False,
        )
        if result.returncode:
            errors.append("PlantUML syntax validation failed")
    elif require_plantuml:
        errors.append("PlantUML is required but the executable is not installed")
    else:
        print("NOTICE: plantuml is not installed; local syntax check skipped")

    if errors:
        print("Documentation validation failed:", file=sys.stderr)
        for error in errors:
            print(f"- {error}", file=sys.stderr)
        return 1

    print(
        f"Documentation validation passed: {len(markdown_candidates())} Markdown "
        f"files, {len(ids)} artifact IDs"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
