"""Unit tests for iteration package current-status validation."""

from __future__ import annotations

import tempfile
import unittest
from pathlib import Path

from scripts.check_docs import validate_iteration_packages


def write_package(
    docs_root: Path,
    milestones: list[tuple[str, str, list[tuple[str, str]]]],
) -> None:
    iteration_root = docs_root / "iterations"
    iteration_root.mkdir(parents=True)
    index_lines = [
        "---",
        "id: TEST-ITERATIONS",
        "artifact: iteration-index",
        "status: current",
        "last_reviewed: 2026-10-16",
        "---",
        "# Iterations",
        "",
    ]
    for milestone_name, milestone_status, iterations in milestones:
        milestone = iteration_root / milestone_name
        milestone.mkdir()
        index_lines.append(f"- [{milestone_name}]({milestone_name}/README.md)")
        milestone_lines = [
            "---",
            f"id: TEST-{milestone_name}",
            "artifact: milestone",
            f"status: {milestone_status}",
            "last_reviewed: 2026-10-16",
            "---",
            f"# {milestone_name}",
            "",
        ]
        for iteration_name, iteration_status in iterations:
            iteration = milestone / iteration_name
            iteration.mkdir()
            milestone_lines.append(f"- [{iteration_name}]({iteration_name}/README.md)")
            (iteration / "README.md").write_text(
                "\n".join(
                    [
                        "---",
                        f"id: TEST-{iteration_name}",
                        "artifact: iteration-plan",
                        f"status: {iteration_status}",
                        "last_reviewed: 2026-10-16",
                        "---",
                        f"# {iteration_name}",
                        "",
                    ]
                ),
                encoding="utf-8",
            )
            (iteration / "WORK_ITEMS.md").write_text("# Work items\n", encoding="utf-8")
        (milestone / "README.md").write_text(
            "\n".join(milestone_lines), encoding="utf-8"
        )
    (iteration_root / "README.md").write_text("\n".join(index_lines), encoding="utf-8")


class ValidateIterationPackagesTests(unittest.TestCase):
    def check(self, milestones: list[tuple[str, str, list[tuple[str, str]]]]) -> list[str]:
        with tempfile.TemporaryDirectory() as temporary:
            docs_root = Path(temporary) / "orchestrator_docs"
            write_package(docs_root, milestones)
            errors: list[str] = []
            validate_iteration_packages(errors, docs_root=docs_root)
            return errors

    def test_completed_iteration_set_allows_zero_current_pair(self) -> None:
        self.assertEqual(self.check([]), [])
        self.assertEqual(
            self.check([("M01-done", "historical", [("I01-01-done", "historical")])]),
            [],
        )

    def test_active_iteration_pair_in_same_milestone_is_valid(self) -> None:
        self.assertEqual(
            self.check([("M01-first", "current", [("I01-01-work", "current")])]),
            [],
        )

    def test_more_than_one_current_milestone_is_invalid(self) -> None:
        errors = self.check(
            [
                ("M01-first", "current", [("I01-01-work", "current")]),
                ("M02-second", "current", []),
            ]
        )
        self.assertTrue(
            any("at most one milestone may be current" in e for e in errors)
        )

    def test_more_than_one_current_iteration_is_invalid(self) -> None:
        errors = self.check(
            [
                (
                    "M01-first",
                    "current",
                    [("I01-01-work", "current"), ("I01-02-next", "current")],
                )
            ]
        )
        self.assertTrue(
            any("at most one iteration plan may be current" in e for e in errors)
        )

    def test_unpaired_current_milestone_or_iteration_is_invalid(self) -> None:
        milestone_only = self.check(
            [("M01-first", "current", [("I01-01-work", "deferred")])]
        )
        iteration_only = self.check(
            [("M01-first", "historical", [("I01-01-work", "current")])]
        )
        self.assertTrue(any("must be paired" in e for e in milestone_only))
        self.assertTrue(any("must be paired" in e for e in iteration_only))

    def test_current_iteration_must_belong_to_current_milestone(self) -> None:
        errors = self.check(
            [
                ("M01-first", "current", [("I01-01-work", "deferred")]),
                ("M02-second", "historical", [("I02-01-work", "current")]),
            ]
        )
        self.assertTrue(any("must belong to the current milestone" in e for e in errors))


if __name__ == "__main__":
    unittest.main()
