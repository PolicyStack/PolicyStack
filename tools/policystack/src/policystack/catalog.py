"""Upstream facts at a release tag, read from git: releases, stops, blocks, elements, examples."""

import itertools
import re
from dataclasses import dataclass
from pathlib import Path

from packaging.version import InvalidVersion, Version

from .repo import Refused, git, load, read

# Every repository needs them (docs/start/install.md).
LOCKED = ("acm", "gitops", "upgrade")
TAG = re.compile(r"v\d+\.\d+\.\d+(-[0-9A-Za-z.]+)?")  # a release, or a prerelease: v1.0.0-rc.1
BLOCKED = "upgrades/blocked.yaml"


def _parse(tag) -> Version | None:
    if TAG.fullmatch(str(tag)):
        try:
            return Version(tag[1:])  # orders v1.0.0-rc.1 < v1.0.0-rc.2 < v1.0.0
        except InvalidVersion:
            pass
    return None


def version(tag: str) -> Version:
    if v := _parse(tag):
        return v
    raise Refused(f"{tag}: not a vX.Y.Z release or vX.Y.Z-rc.N prerelease")


def prerelease(tag) -> bool:
    return bool((v := _parse(tag)) and v.is_prerelease)


def tags(gitdir: Path, pre: bool = False) -> list[str]:
    """Releases, oldest first; prereleases too with pre."""
    found = sorted(
        (t for t in git("tag", "-l", "v*", cwd=gitdir).split() if _parse(t)), key=version
    )
    if out := [t for t in found if pre or not prerelease(t)]:
        return out
    url = git("config", "remote.origin.url", cwd=gitdir, ok=(1,)).strip() or gitdir
    hint = f"; prereleases: {', '.join(found)} (pass --pre, or --to one of them)" if found else ""
    raise Refused(f"{url}: no vX.Y.Z release tags{hint}")


def steps(gitdir: Path, tag: str) -> dict[str, dict]:
    """upgrades/<name>.yaml at tag, by name. blocked.yaml is not a step; no upgrades/ is none."""
    names = git("ls-tree", "--name-only", tag, "upgrades/", cwd=gitdir).split()
    blobs = read(gitdir, tag, [n for n in names if n.endswith(".yaml") and n != BLOCKED])
    return {Path(n).name: load(b) for n, b in sorted(blobs.items())}


def blocked(gitdir: Path, newest: str) -> dict[str, str]:
    """tag -> reason, from upgrades/blocked.yaml at the newest release: a tag can't record a
    block found after it shipped, because tags never move."""
    data = read(gitdir, newest, [BLOCKED]).get(BLOCKED, b"[]")
    return {
        b["to"]: b["message"] + (f"; use {b['fixedIn']}" if b.get("fixedIn") else "")
        for b in load(data) or []
    }


def stops(gitdir: Path, releases: list[str], block: dict[str, str]) -> dict[str, str]:
    """Releases an update must land on before going further: the last release of each major
    (majors go one at a time), and the first release that ships a step with stop: true. A
    blocked release is never a stop: its role moves to the newest unblocked release before it,
    or the first after it."""
    releases = [t for t in releases if t not in block]
    majors = sorted({version(t).major for t in releases})
    last = {version(t).major: t for t in releases if not prerelease(t)}  # a stop is never an rc
    out = {
        last[a]: f"last v{a} release before v{b}"
        for a, b in itertools.pairwise(majors)
        if a in last
    }
    seen: set[str] = set()
    for t in releases:
        for n, s in steps(gitdir, t).items():
            if n not in seen and s.get("stop"):
                out[t] = "; ".join(filter(None, (out.get(t), s["summary"])))
            seen.add(n)
    return out


def targets(releases, current: str | None, stop: dict[str, str], block: dict[str, str]):
    """(tag, disabled reason or None, note) for every release after current, newest first.
    With no current (install) every release is listed and stops don't apply."""
    rows, held = [], None
    for t in releases:
        if current and version(t) <= version(current):
            continue
        why = [f"update to {held} first (required stop)"] if held else []
        why += [f"blocked: {block[t]}"] if t in block else []
        note = f"required stop: {stop[t]}" if current and t in stop else ""
        rows.append((t, "; ".join(why) or None, note))
        held = held or (note and t)
    return rows[::-1]


@dataclass(frozen=True)
class Element:
    name: str
    description: str
    stability: str
    hard: tuple[str, ...]  # named by a policy enabled by default
    optional: tuple[str, ...]  # named only by policies that ship enabled: false


def camel(name: str) -> str:
    """The element's key under stack: in values files: cert-manager -> certManager."""
    head, *rest = name.split("-")
    return head + "".join(p[:1].upper() + p[1:] for p in rest)


def deps(name: str, values: dict) -> tuple[tuple[str, ...], tuple[str, ...]]:
    """policies.*.dependencies[].element, split by whether the naming policy is enabled by
    default: anything but false or "false" is enabled, as in policy-library.enabled."""
    hard, optional = set(), set()
    for comp in ((values or {}).get("stack") or {}).values():
        for policy in (comp.get("policies") or {}).values():
            for d in (policy or {}).get("dependencies") or []:
                if d.get("element") and d["element"] != name:
                    v = policy.get("enabled")
                    (optional if v is False or v == "false" else hard).add(d["element"])
    return tuple(sorted(hard)), tuple(sorted(optional - hard))


def names(gitdir: Path, rev: str) -> list[str]:
    """Element directories under stack/ at rev."""
    out = git("ls-tree", "-d", "--name-only", rev, "stack/", cwd=gitdir)
    return [Path(p).name for p in out.split()]


def elements(gitdir: Path, tag: str) -> dict[str, Element]:
    dirs = names(gitdir, tag)
    files = [f"stack/{n}/{f}" for n in dirs for f in ("Chart.yaml", "values.yaml")]
    blobs = read(gitdir, tag, files)
    out = {}
    for n in dirs:
        chart = load(blobs[f"stack/{n}/Chart.yaml"])
        notes = chart.get("annotations") or {}
        stability = notes.get("policystk.io/stability", "preview")
        values = load(blobs.get(f"stack/{n}/values.yaml", b"{}"))
        out[n] = Element(n, chart.get("description", ""), stability, *deps(n, values))
    return out


def examples(gitdir: Path, tag: str, catalog: dict[str, Element]) -> dict[str, tuple]:
    """values/** at tag -> the elements each file turns on (stack.<key>.enabled: true)."""
    paths = git("ls-tree", "-r", "--name-only", tag, "values/", cwd=gitdir).split()
    by_key = {camel(n): n for n in catalog}
    out = {}
    for p, data in read(gitdir, tag, paths).items():
        doc = load(data) if p.endswith((".yaml", ".yml")) else None
        stack = (doc or {}).get("stack") or {}
        on = {k for k, v in stack.items() if isinstance(v, dict) and v.get("enabled") is True}
        out[p] = tuple(sorted(by_key[k] for k in on & by_key.keys()))
    return out


def closure(catalog: dict[str, Element], picked: set[str]) -> tuple[set, dict[str, str]]:
    """picked plus every hard dependency, transitively; returns (all, {added: needed by})."""
    out, why, todo = set(picked), {}, sorted(picked)
    while todo:
        el = todo.pop()
        for d in catalog[el].hard:
            if d not in out:
                out.add(d)
                why[d] = el
                todo.append(d)
    return out, why
