#!/usr/bin/env python3
"""Structural validation for the public fixture bundle."""

from __future__ import annotations

import json
import sys
from pathlib import Path

import yaml


def load(path: Path):
    with path.open(encoding="utf-8") as f:
        return yaml.safe_load(f)


def unescape(token: str) -> str:
    return token.replace("~1", "/").replace("~0", "~")


def apply_patch(doc, operations):
    import copy
    out = copy.deepcopy(doc)
    for op in operations:
        tokens = [unescape(x) for x in op["path"].split("/")[1:]]
        parent = out
        for token in tokens[:-1]:
            parent = parent[int(token)] if isinstance(parent, list) else parent[token]
        key = tokens[-1]
        if op["op"] == "remove":
            parent.pop(int(key)) if isinstance(parent, list) else parent.pop(key)
        elif op["op"] == "add":
            if isinstance(parent, list):
                parent.append(op["value"]) if key == "-" else parent.insert(int(key), op["value"])
            else: parent[key] = op["value"]
        elif op["op"] == "replace":
            if isinstance(parent, list): parent[int(key)] = op["value"]
            else: parent[key] = op["value"]
        else: raise AssertionError(f"unsupported op {op['op']}")
    return out


def apply_delta(current, delta):
    import copy
    out = copy.deepcopy(current)
    out.setdefault("modules", {})
    md = delta.get("modules", {})
    for k, v in md.get("add", {}).items(): out["modules"][k] = v
    for k in md.get("remove", []): out["modules"].pop(k)
    for k, p in md.get("update", {}).items(): out["modules"][k] = apply_patch(out["modules"][k], p)
    if "shared" in delta:
        out["shared"] = apply_patch(out.get("shared", {}), delta["shared"])
        if not out["shared"]: out.pop("shared")
    return out


def validate_case(root: Path):
    manifest = load(root / "case.yaml")
    assert manifest["apiVersion"] == "challenge.humanitec.io/v1alpha1"
    assert manifest["kind"] == "PlannerCase"
    spec = manifest["spec"]
    for key in ("context", "currentDeploymentSet", "activeResources"):
        assert (root / spec[key]).is_file(), (root.name, key)
    assert spec["beforeScore"] is not None or spec["afterScore"] is not None
    names = []
    for key in ("beforeScore", "afterScore"):
        if spec[key] is not None:
            p = root / spec[key]
            assert p.is_file()
            score = load(p)
            assert score["apiVersion"] == "score.dev/v1b1"
            names.append(score["metadata"]["name"])
    assert len(set(names)) <= 1

    for rel in spec["resourceTypes"]:
        d = load(root / rel)
        assert d["apiVersion"] == "entity.humanitec.io/v1b1"
        assert d["kind"] == "ResourceType"
    for rel in spec["resourceDefinitions"]:
        d = load(root / rel)
        assert d["apiVersion"] == "entity.humanitec.io/v1b1"
        assert d["kind"] == "Definition"
        assert "criteria" in d["entity"]
        assert "criteria" not in d
    for mapping in spec["terraformSourceMap"]:
        checkout = root / mapping["directory"]
        assert checkout.is_dir()
        assert list(checkout.rglob("*.tf"))
        assert not list(checkout.rglob("contract.json"))

    result = json.loads((root / "expected/result.json").read_text(encoding="utf-8"))
    if result["status"] == "ACCEPTED":
        assert load(root / "expected/delta.yaml") == result["delta"]
        assert load(root / "expected/deployment-set.yaml") == result["deploymentSet"]
        assert load(root / "expected/challenge-plan.yaml") == result["challengePlan"]
        current = load(root / spec["currentDeploymentSet"])
        assert apply_delta(current, result["delta"]) == result["deploymentSet"]
    else:
        assert load(root / "expected/error.yaml") == result["error"]


def main():
    bundle = Path(__file__).resolve().parent
    cases = sorted(p for p in (bundle / "testcases").iterdir() if p.is_dir())
    for case in cases: validate_case(case)
    print(f"validated {len(cases)} cases")


if __name__ == "__main__":
    try: main()
    except Exception as exc:
        print(f"validation failed: {exc}", file=sys.stderr)
        raise

