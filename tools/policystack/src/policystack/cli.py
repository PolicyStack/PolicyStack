"""policystack install | add | update: copier's public API in process, one answers file per scope.

Needs git 2.43 or later (merge-file --object-id). From mise: mise run ps -- --help.
"""

import argparse
import difflib
import shlex
import shutil
import sys
from pathlib import Path

import questionary
from copier.errors import UserMessageError
from questionary import Choice

from . import catalog as cat
from . import migrate, repo
from .repo import ANSWERS, Refused, git

URL = "https://github.com/PolicyStack/PolicyStack.git"
HUB = (
    "hub chart changed: before merging, run on every hub from this branch:\n"
    "  helm upgrade policystack ./install -n openshift-gitops --reset-then-reuse-values"
)
MAJOR = (
    "major release: follow docs/guides/upgrading.md#majors (every hub from this branch, then merge)"
)
OPTIONAL = "named only by policies that ship disabled; POLICY011 fails once a cluster enables one"


def _tty() -> bool:
    return sys.stdin.isatty() and sys.stdout.isatty()


def _need(what: str) -> None:
    """Without a terminal nothing is asked: prompt_toolkit would read a pipe as keystrokes."""
    if not _tty():
        raise Refused(f"no terminal (stdin and stdout must both be one): {what}")


def _csv(value: str) -> list[str]:
    return [v.strip() for v in value.split(",") if v.strip()]


def _pick_tag(rows) -> str:
    choices = [Choice(f"{t}  {note}".rstrip(), value=t, disabled=why) for t, why, note in rows]
    return questionary.select("Release", choices, use_jk_keys=False).unsafe_ask()


def _check_tag(tag: str, rows) -> None:
    row = next((r for r in rows if r[0] == tag), None)
    if row is None or row[1]:
        valid = ", ".join(r[0] for r in rows if not r[1]) or "none"
        why = row[1] if row else "not a release you can move to"
        raise Refused(f"{tag}: {why} (you can move to: {valid})")


def _checkbox(message: str, rows, locked=(), why: str = "") -> list[str]:
    """rows: (value, facts, description, checked). The filter searches the title, so it holds
    the name and short facts only. Locked rows stay ticked through validate, because disabled=
    breaks questionary's filter (the pointer lands on, and toggles, a disabled row)."""
    label = {v: v + " (locked)" * (v in locked) for v, *_ in rows}
    pad = max(map(len, label.values()), default=0)
    width = shutil.get_terminal_size().columns - 6
    title = {v: f"{label[v]:<{pad}}  {facts}".rstrip()[:width] for v, facts, *_ in rows}
    choices = [
        Choice(title[v], v, checked=c or v in locked, description=d or None) for v, _, d, c in rows
    ]
    return questionary.checkbox(
        message,
        choices,
        use_search_filter=True,
        use_jk_keys=False,
        instruction="(type to filter by name, <space> toggle, <enter> done)",
        validate=lambda s: set(locked) <= set(s) or why,
    ).unsafe_ask()


def _element_rows(elements: dict[str, cat.Element], names) -> list:
    rows = []
    for e in (elements[n] for n in sorted(names)):
        needs = f"needs {', '.join(e.hard)}. " if e.hard else ""
        rows.append((e.name, e.stability, needs + e.description, False))
    return rows


def _confirm() -> None:
    if _tty() and not questionary.confirm("Proceed?", default=False).unsafe_ask():
        raise Refused("cancelled; nothing changed")


def _unknown(asked, known, what: str) -> None:
    if bad := sorted(set(asked) - set(known)):
        near = {b: difflib.get_close_matches(b, known, 1) for b in bad}
        named = (b + "".join(f" (did you mean {m}?)" for m in near[b]) for b in bad)
        raise Refused(f"unknown {what}: {', '.join(named)}")


def _summary(elements, final, why, implied, have=frozenset()) -> None:
    print(f"  elements: {', '.join(sorted(final))}")
    added = {**{d: f"needed by {n}" for d, n in why.items()}, **implied}
    for el, reason in sorted(added.items()):
        print(f"    + {el} {reason}")
    have = final | have
    if off := sorted(f"{e} -> {d}" for e in final for d in set(elements[e].optional) - have):
        print(f"  not added, {OPTIONAL}: {', '.join(off)}")


def _commit(root: Path, scope: str) -> str:
    tag = repo.answers(root, scope)["_commit"]
    if not cat.TAG.fullmatch(str(tag)):
        raise Refused(
            f"{Path(scope, ANSWERS)}: _commit {tag} is not a release tag; set it to "
            "the release that scope was copied from"
        )
    return tag


def _open(need: str | None) -> tuple[Path, str, str, Path]:
    """The clean repository at the working directory, its upstream URL, its framework release
    and the mirror of upstream. need: what to pass without a terminal."""
    root = Path(git("rev-parse", "--show-toplevel", cwd=Path.cwd()).strip())
    if not (root / ANSWERS).is_file():
        raise Refused(f"not a PolicyStack repository (no {ANSWERS} at {root})")
    repo.require_clean(root)
    if need:
        _need(need)
    url, tag = repo.answers(root, ".")["_src_path"], _commit(root, ".")
    return root, url, tag, repo.mirror(url)


def _present(root: Path) -> set[str]:
    return {p.name for p in (root / "stack").iterdir() if p.is_dir()}


def _copy(url: str, mirror: Path, tag: str, root: Path, scopes) -> None:
    for i, scope in enumerate(scopes, 1):
        print(f"copying {repo.element(scope, 'framework')} ({i}/{len(scopes)})")
        repo.copy(url, mirror, tag, root, scope)


def install(a) -> None:
    dst = Path(a.dir).absolute()
    if dst.exists() and any(dst.iterdir()):
        raise Refused(f"{dst}: exists and is not empty")
    parent = next(p for p in (dst, *dst.parents) if p.exists())
    if top := git("rev-parse", "--show-toplevel", cwd=parent, ok=(128,)).strip():
        raise Refused(f"{dst}: inside the git work tree {top}; install makes its own repository")
    if a.elements is None:
        _need("pass --elements (comma-separated; may be empty)")
    mirror = repo.mirror(a.src)
    releases = cat.tags(mirror, a.pre or cat.prerelease(a.to))
    rows = cat.targets(releases, None, {}, cat.blocked(mirror, releases[-1]))
    newest = next((t for t, why, _ in rows if not why), "")
    tag = a.to or (_pick_tag(rows) if _tty() else newest)
    _check_tag(tag, rows)
    repo.peel(mirror, tag)  # refuse a double-tagged release before anything is written
    elements = cat.elements(mirror, tag)
    examples = cat.examples(mirror, tag, elements)
    if a.elements is None:
        locked = f"{', '.join(cat.LOCKED)} are always installed"
        rows = _element_rows(elements, elements)
        a.elements = _checkbox("Elements", rows, cat.LOCKED, locked)
    if a.examples is None and _tty():
        rows = [(p, "", f"enables {', '.join(e) or 'nothing'}", False) for p, e in examples.items()]
        a.examples = _checkbox("Example values files (yours once copied)", rows)
    picked, files = set(a.elements), a.examples or []
    _unknown(picked, elements, "elements")
    _unknown(files, examples, "example files")
    implied = {e: f"enabled by {f}" for f in files for e in set(examples[f]) - picked}
    final, why = cat.closure(elements, picked | set(cat.LOCKED) | implied.keys())
    print(f"PolicyStack {tag} into {dst}")
    print("  framework: install/, lifecycle/, lib/, tools/, CI, mise.toml")
    _summary(elements, final, why, implied)
    if files:
        print(f"  example files, yours from now on: {', '.join(files)}")
    _confirm()
    try:  # all or nothing: a failure or Ctrl-C removes what this run created
        dst.mkdir(parents=True, exist_ok=True)
        git("init", "-q", "-b", "main", cwd=dst)
        _copy(a.src, mirror, tag, dst, [".", *(f"stack/{e}" for e in sorted(final))])
        for path, data in repo.read(mirror, tag, files).items():
            repo.write(dst / path, data)
        git("add", "-A", cwd=dst)
        git("commit", "-q", "-m", f"chore: PolicyStack {tag}", cwd=dst)
    except BaseException:
        with repo.undoing():
            shutil.rmtree(dst, ignore_errors=True)
        raise
    print(
        f"created {dst}: one commit on main, nothing pushed. Next (docs/start/install.md):\n"
        f"  git -C {dst} remote add origin <your repository URL> && git -C {dst} push -u origin main\n"
        "  write fleet/hubs/<hub>.yaml and fleet/<cluster>.yaml, then: mise run validate"
    )


def add(a) -> None:
    need = None if a.elements else "name the elements: mise run ps -- add <element>..."
    root, url, tag, mirror = _open(need)
    elements, present = cat.elements(mirror, tag), _present(root)
    rows = _element_rows(elements, set(elements) - present)
    picked = set(a.elements or _checkbox("Elements to add", rows))
    missing = picked - set(elements)
    later = {}  # element -> the first release after tag that has it
    for t in cat.tags(mirror, cat.prerelease(tag)) if missing else []:
        if cat.version(t) > cat.version(tag):
            later |= {e: t for e in missing & set(cat.names(mirror, t)) - later.keys()}
    if later:
        first = "; ".join(f"{e} first ships in {t}" for e, t in sorted(later.items()))
        newest = max(later.values(), key=cat.version)
        raise Refused(f"{first}; update the framework first: mise run update --to {newest}")
    _unknown(picked, elements, "elements")
    if have := sorted(picked & present):
        raise Refused(f"already in stack/: {', '.join(have)}")
    final, why = cat.closure(elements, picked)
    new = sorted(final - present)
    print(f"add at {tag} (your framework release)")
    _summary(elements, set(new), {d: why[d] for d in new if d in why}, {}, present)
    _confirm()
    paths = tuple(f"stack/{e}" for e in new)
    with repo.scope_run(root, "add", paths):
        _copy(url, mirror, tag, root, paths)
        repo.commit(root, paths, f"feat: add {', '.join(new)} ({tag})")
    print("committed; nothing pushed")


def _detail(root: Path, mirror: Path, old: str, new: str, scope: str, moved) -> str:
    mine = repo.modified(root, mirror, old, scope)
    overlap = mine & {p for c in moved for p in c[1:]}
    out = [f"{len(moved)} file{'s' * (len(moved) != 1)} changed"]
    out += [f"you modified {len(mine)}"] if mine else []
    out += [f"{len(overlap)} overlap -> may conflict"] if overlap else []
    out += ["BREAKING"] if repo.breaking(mirror, old, new, scope) else []
    return ", ".join(out)


def _plan(root: Path, mirror: Path, tracked: list[str], tag: str):
    """(elements at tag, changed rows, unchanged, removed upstream) for tracked elements, from
    git only. A changed row is (element, detail, description, moved by a major)."""
    elements, changed, unchanged, removed = cat.elements(mirror, tag), [], [], []
    for el in tracked:
        scope, old = f"stack/{el}", _commit(root, f"stack/{el}")
        if el not in elements:
            removed.append(el)
            continue
        moved = repo.changes(mirror, old, tag, scope) if old != tag else []
        # Elements share the framework's major.
        major = cat.version(old).major < cat.version(tag).major
        if moved or major:
            detail = _detail(root, mirror, old, tag, scope, moved)
            changed.append((el, detail, elements[el].description, major))
        else:
            unchanged.append(el)
    return elements, changed, unchanged, removed


def update(a) -> int:
    root, _, current, mirror = _open(None if a.to else "pass --to <release>")
    releases = cat.tags(mirror, a.pre or cat.prerelease(a.to))
    block = cat.blocked(mirror, releases[-1])
    rows = cat.targets(releases, current, cat.stops(mirror, releases, block), block)
    rows.append((current, None, "current: elements behind it only"))
    print(f"current: {current}")
    tag = a.to or (current if len(rows) == 1 else _pick_tag(rows))
    _check_tag(tag, rows)
    repo.peel(mirror, tag)
    branch = f"policystack/update-{tag}"
    resume = git("branch", "--show-current", cwd=root).strip() == branch
    if not resume and git("branch", "--list", branch, cwd=root).strip():
        raise Refused(
            f"{branch} exists. To finish it: git switch {branch}, then rerun. "
            f"To start over: git branch -D {branch}"
        )
    tracked = sorted(p.parent.name for p in root.glob(f"stack/*/{ANSWERS}"))
    elements, changed, unchanged, removed = _plan(root, mirror, tracked, tag)
    major = cat.version(current).major < cat.version(tag).major
    if major and removed:
        raise Refused(
            f"removed upstream by {tag}: {', '.join(removed)}. POLICY061 fails while "
            f"they stay on v{cat.version(current).major}: delete stack/<element>, or only "
            f"its {ANSWERS} to keep it as yours, commit, then rerun"
        )
    applied = cat.steps(mirror, current)
    pending = [(n, s) for n, s in cat.steps(mirror, tag).items() if n not in applied]
    names = [c[0] for c in changed]
    forced = {c[0]: "a major moves every element" for c in changed if c[3]}
    # A step rewrites an element's keys, so that element must move with it.
    for n, s in pending:
        keys = migrate.stack_keys(s.get("ops") or [])
        for e in names:
            if "*" in keys or cat.camel(e) in keys:
                forced.setdefault(e, f"upgrade step {n} rewrites its keys")
    if a.elements:
        _unknown(a.elements, tracked, "elements (not installed)")
        if same := sorted(set(a.elements) - set(names)):
            raise Refused(f"nothing to update in {', '.join(same)}: unchanged upstream up to {tag}")
        if left := sorted(set(forced) - set(a.elements)):
            raise Refused(
                "these must move in this update too: "
                + "; ".join(f"{e} ({forced[e]})" for e in left)
            )
    if a.elements or not _tty() or set(forced) >= set(names):
        picked = a.elements or names
    else:
        rows = [(el, d, desc, True) for el, d, desc, _ in changed]
        why = "; ".join(f"{e} ({r})" for e, r in forced.items())
        why = f"locked rows must move: {why}"
        picked = _checkbox("Elements to update", rows, forced, why)
    files = set(git("ls-files", "-z", "--", *migrate.ADOPTER, cwd=root).split("\0"))
    try:
        texts = migrate.apply(root, pending, files)
    except ValueError as e:
        raise Refused(f"upgrade step cannot be applied, nothing changed:\n{e}") from e
    if current == tag and not picked:
        print("up to date: nothing changed upstream for the installed elements")
        return 0
    framework = repo.changes(mirror, current, tag, ".") if current != tag else []
    hub = any(p.startswith("install/") for c in framework for p in c[1:])
    if resume and not hub:  # the framework commit this branch may already have
        done = f"--grep=chore(policystack): update framework to {tag}"
        log = ("log", "-1", "--format=%h", "-F", done, "--", "install/")
        hub = bool(git(*log, cwd=root))
    if current == tag:
        print(f"PolicyStack {tag}: elements behind it")
    else:
        print(f"PolicyStack {current} -> {tag}")
        if major:
            print(f"  {MAJOR}")
        print(f"  framework: {_detail(root, mirror, current, tag, '.', framework)}")
    for n, s in pending:
        print(f"  upgrade step {n}: {s['summary']}")
        if s.get("manual") and not s.get("ops"):
            when = "before the next release" if s.get("stop") else "by hand"
            print(f"    {when}: " + s["manual"].rstrip().replace("\n", "\n      "))
    paths = tuple(str(p.relative_to(root)) for p in texts)
    if paths:
        print(f"  upgrade steps rewrite: {', '.join(paths)}")
    for el, detail, *_ in changed:
        print(f"  [{'x' if el in picked else ' '}] {el:<20} {detail}")
    present, before = _present(root), set(cat.names(mirror, current))
    for label, group in (
        ("unchanged", unchanged),
        ("removed upstream (yours to delete)", removed),
        ("new upstream (mise run ps -- add)", sorted(set(elements) - before - present)),
        ("untracked (yours)", sorted(present - set(tracked))),
    ):
        if group:
            print(f"  {label}: {', '.join(group)}")
    for el in picked:
        if missing := sorted(set(elements[el].hard) - present):
            print(f"  note: {el} at {tag} needs: mise run ps -- add {' '.join(missing)}")
        if off := sorted(set(elements[el].optional) - present):
            print(f"  note: {el} at {tag}: {', '.join(off)} {OPTIONAL}")
    if hub:
        print(f"  {HUB}")
    _confirm()
    start = git("rev-parse", "HEAD", cwd=root).strip()
    if not resume:
        git("switch", "-q", "-c", branch, cwd=root)
    try:
        if paths:
            with repo.scope_run(root, "upgrade steps", paths):
                for path, text in texts.items():
                    path.write_bytes(text.encode())
                repo.commit(root, paths, f"chore(policystack): upgrade steps for {tag}")
        todo = (["."] if current != tag else []) + [f"stack/{el}" for el in picked]
        for i, scope in enumerate(todo, 1):
            name, spec = repo.element(scope, "framework"), repo.pathspec(scope)
            message = f"chore(policystack): update {name} to {tag}"
            print(f"updating {name} ({i}/{len(todo)})")
            with repo.scope_run(root, name, spec):
                if not (stuck := repo.update(root, mirror, scope, tag)):
                    repo.commit(root, spec, message)
            if stuck:
                rest = (
                    [repo.element(s, "framework") for s in todo[i:]]
                    if set(picked) != set(names)
                    else []
                )
                print(f"stopped: conflicts in {name}:", *stuck, sep="\n  ")
                print("resolve each file (git rm one you no longer need), then:")
                add = f"git add -A -- {shlex.join(spec)}"
                print(f"  {add} && git commit -m {shlex.quote(message)}")
                print(f"  mise run update --to {tag} {' '.join(rest)}".rstrip())
                return 1
        print(f"done on branch {branch}, nothing pushed" + (f"\n{HUB}" if hub else ""))
        return 0
    except BaseException:  # refused or interrupted: drop a branch that never got a commit
        unchanged = git("rev-parse", "HEAD", cwd=root).strip() == start
        if not resume and unchanged and not git("status", "--porcelain", cwd=root):
            git("switch", "-q", "-", cwd=root)
            git("branch", "-q", "-D", branch, cwd=root)
        raise


def main(argv: list[str] | None = None) -> int:
    # Progress lines show as they happen, and in order with stderr.
    sys.stdout.reconfigure(line_buffering=True)
    p = argparse.ArgumentParser(
        prog="policystack",
        description=__doc__.splitlines()[0],
        epilog="From mise: mise run ps -- --help. Runs from the repository root.",
    )
    sub = p.add_subparsers(dest="cmd", required=True)
    i = sub.add_parser("install", help="create a new repository from a release")
    i.add_argument("dir", help="directory for the new repository (empty or absent)")
    i.add_argument("--src", default=URL, help="upstream URL (default %(default)s)")
    i.add_argument("--to", help="release; default: pick one, or the newest")
    i.add_argument("--pre", action="store_true", help="also list prereleases (vX.Y.Z-rc.N)")
    i.add_argument("--elements", type=_csv, help="comma-separated element names")
    i.add_argument("--examples", type=_csv, help="comma-separated values/ files")
    s = sub.add_parser("add", help="add elements at the framework's release")
    s.add_argument("elements", nargs="*")
    u = sub.add_parser("update", help="update on branch policystack/update-<release>")
    u.add_argument("--to", help="release")
    u.add_argument("--pre", action="store_true", help="also list prereleases (vX.Y.Z-rc.N)")
    u.add_argument("elements", nargs="*", help="default: every changed element")
    a = p.parse_args(argv)
    a.to = getattr(a, "to", None) or None  # --to '' is not a release
    try:
        return {"install": install, "add": add, "update": update}[a.cmd](a) or 0
    except (Refused, UserMessageError) as e:
        print(f"policystack: {e}", file=sys.stderr)
        return 1
    except KeyboardInterrupt:
        print("cancelled; anything half done was undone", file=sys.stderr)
        return 130
