"""git and copier on the adopter repository and on the launcher's mirror of upstream.

A scope is "." (the framework) or "stack/<element>". Upstream paths equal adopter paths in both:
the framework copies to the repository root and an element's _subdirectory is its destination.
"""

import contextlib
import errno
import hashlib
import io
import json
import os
import re
import shlex
import shutil
import signal
import subprocess
import tempfile
from pathlib import Path

from copier import Settings, run_copy, run_update
from ruamel.yaml import YAML

ANSWERS = ".copier-answers.yml"
OWNED = ("stack/", "values/", "fleet/")  # never in the framework scope's commit
# copier's conflict labels, so the F1 merge reads like the conflicts copier writes
LABELS = ("-L", "before updating", "-L", "last update", "-L", "after updating")
MARKER = "^<{7} "  # any conflict marker's first line, as a git grep -E pattern
EMPTY_TREE = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"
FRAMEWORK_ONLY = re.compile(r"\{% if not element %\}(.*?)\{% endif %\}", re.DOTALL)
BREAKING = re.compile(r"[\w-]+(\([^)]*\))?(!: |: breaking: )|breaking: ")


class Refused(Exception):
    """A precondition failed; nothing was changed by the step that raised it."""


def run(*args: str, cwd: Path, stdin: bytes | None = None, ok=()):
    """git with args; an exit code other than 0 or one in ok is a Refused."""
    p = subprocess.run(["git", *args], cwd=cwd, input=stdin, capture_output=True, check=False)
    if p.returncode and p.returncode not in ok:
        raise Refused(f"git {' '.join(args)}: {p.stderr.decode().strip()}")
    return p


def git(*args: str, cwd: Path, stdin: bytes | None = None, ok=()) -> str:
    return run(*args, cwd=cwd, stdin=stdin, ok=ok).stdout.decode()


def read(gitdir: Path, rev: str, paths: list[str]) -> dict[str, bytes]:
    """Blobs at rev, one cat-file call; missing paths are left out."""
    query = "".join(f"{rev}:{p}\n" for p in paths).encode()
    out = run("cat-file", "--batch", cwd=gitdir, stdin=query).stdout
    blobs, i = {}, 0
    for p in paths:
        nl = out.index(b"\n", i)
        head = out[i:nl].split()
        if head[-1] == b"missing":
            i = nl + 1
            continue
        size = int(head[2])
        blobs[p], i = out[nl + 1 : nl + 1 + size], nl + 2 + size
    return blobs


def load(data: bytes | str):
    return YAML(typ="safe", pure=True).load(data)


def write(path: Path, data: bytes) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_bytes(data)


def mirror(url: str) -> Path:
    """The launcher's bare copy of upstream tags, fetched through the adopter's url.insteadOf.
    Tags fetch without force or prune (whatever fetch.prune says), so a tag that moved, or was
    deleted and made again elsewhere, is refused for as long as this cache lives."""
    cache = Path(os.environ.get("XDG_CACHE_HOME") or Path.home() / ".cache")
    path = cache / "policystack" / f"{hashlib.sha256(url.encode()).hexdigest()[:16]}.git"
    if not path.exists():
        path.mkdir(parents=True)
        git("init", "-q", "--bare", cwd=path)
        git("remote", "add", "origin", url, cwd=path)
        git("config", "remote.origin.fetch", "refs/tags/*:refs/tags/*", cwd=path)
    try:
        git("fetch", "--no-tags", "--no-prune", "--no-prune-tags", "origin", cwd=path)
    except Refused as e:
        if "would clobber existing tag" in str(e):
            raise Refused(
                "upstream moved a released tag; tags never move, so this is refused. Ask "
                f"upstream; if the new tag is right, delete {path} and rerun.\n{e}"
            ) from e
        raise
    return path


def peel(mirror: Path, tag: str) -> str:
    """The commit copier renders: the one this mirror verified, not whatever the tag names
    upstream by the time copier fetches. copier records _commit with `git describe --tags`, so
    any other tag on that commit could be recorded instead of the release."""
    named = git("describe", "--tags", "--exact-match", tag, cwd=mirror).strip()
    if named != tag:
        raise Refused(
            f"{tag}: copier would record {named} for this commit; a release commit carries "
            "its vX.Y.Z tag only"
        )
    return git("rev-parse", f"{tag}^{{commit}}", cwd=mirror).strip()


def answers(root: Path, scope: str) -> dict:
    return load((root / scope / ANSWERS).read_text())


def pathspec(scope: str) -> tuple[str, ...]:
    """What a scope's commit stages."""
    return (".", *(f":(exclude){p}" for p in OWNED)) if scope == "." else (scope,)


def _copier_yml(mirror: Path, tag: str) -> str:
    return read(mirror, tag, ["copier.yml"])["copier.yml"].decode()


def spec(mirror: Path, tag: str, scope: str) -> tuple[str, ...]:
    """Upstream paths the scope copies at tag: the element directory, or the root less
    copier.yml and its framework-only excludes (stack, values, fleet, docs, README.md, ...)."""
    if scope != ".":
        return (scope,)
    block = FRAMEWORK_ONLY.search(_copier_yml(mirror, tag))[1]
    return (
        ".",
        *(f":(exclude){p.lstrip('/')}" for p in ["copier.yml", *block.split()]),
    )


def changes(mirror: Path, old: str, new: str, scope: str) -> list[list[str]]:
    """Upstream name-status between two tags with renames: ["M", path], ["D", path],
    ["R097", old, new]. An element lists copier.yml only when it changed outside the
    framework-only block."""
    paths = spec(mirror, new, scope)
    out = git("diff", "--name-status", "-M", old, new, "--", *paths, cwd=mirror)
    rows = [line.split("\t") for line in out.splitlines()]
    shared = {FRAMEWORK_ONLY.sub("", _copier_yml(mirror, t)) for t in (old, new)}
    return rows + [["M", "copier.yml"]] if scope != "." and len(shared) > 1 else rows


def breaking(mirror: Path, old: str, new: str, scope: str) -> bool:
    """A commit touching the scope is breaking: a Conventional Commit '!', or a subject (or its
    description) starting with 'breaking:' (compatibility.md: preview breaks, unfixable
    security fixes). A multi-scope commit flags each scope it touches."""
    paths = spec(mirror, new, scope)
    subjects = git("log", "--format=%s", f"{old}..{new}", "--", *paths, cwd=mirror)
    return any(BREAKING.match(s) for s in subjects.splitlines())


def tree(gitdir: Path, rev: str, paths: tuple[str, ...]) -> dict[str, tuple[str, str]]:
    """path -> (mode, blob id) at rev. A diff against the empty tree, since ls-tree takes no
    exclude pathspecs."""
    args = ("--raw", "-z", "--no-renames", "--no-abbrev", EMPTY_TREE, rev, "--", *paths)
    f = git("diff", *args, cwd=gitdir).split("\0")
    return {p: (meta.split()[1], meta.split()[3]) for meta, p in zip(f[0::2], f[1::2])}


def _trees(root: Path, mirror: Path, old: str, scope: str):
    paths = spec(mirror, old, scope)
    return tree(mirror, old, paths), tree(root, "HEAD", paths)


def _mine(base: dict, head: dict) -> set[str]:
    """Paths in base that head changed or lacks. *.jinja are the answers-file templates, which a
    copy never holds under that name."""
    return {
        p
        for p, (_, sha) in base.items()
        if head.get(p, ("", ""))[1] != sha and not p.endswith(".jinja")
    }


def modified(root: Path, mirror: Path, old: str, scope: str) -> set[str]:
    """Files copied at old that the adopter edited or deleted since: blob ids only, no render."""
    return _mine(*_trees(root, mirror, old, scope))


def unmerged(root: Path) -> list[str]:
    return sorted({line.split("\t")[1] for line in git("ls-files", "-u", cwd=root).splitlines()})


def _marker_file(root: Path) -> Path:
    return root / git("rev-parse", "--git-path", "policystack-update", cwd=root).strip()


def _dirty(root: Path) -> bool:
    args = ("--porcelain", "--untracked-files=all", "--ignore-submodules=none")
    return bool(git("status", *args, cwd=root))


def require_clean(root: Path) -> None:
    """Whole repository, not copier's subdirectory-only check: an uncommitted change in another
    scope would otherwise be resurrected or swept into this scope's commit. Untracked files count
    whatever status.showUntrackedFiles says. Also refuses while an earlier run's scope is half
    applied (the marker outlived it) and while a committed file holds a conflict marker."""
    if (mark := _marker_file(root)).exists():
        m = json.loads(mark.read_text())
        on = git("branch", "--show-current", cwd=root).strip() == m["branch"]
        if on and git("rev-parse", "HEAD", cwd=root).strip() == m["head"] and not _dirty(root):
            mark.unlink()
        elif on:
            raise Refused(
                f"an earlier run stopped while updating {m['name']}, leaving it half applied. "
                f"Undo it, then rerun:\n  git reset -q --hard {m['head']} && "
                f"git clean -fdq -- {shlex.join(m['paths'])}"
            )
        elif git("branch", "--list", m["branch"], cwd=root).strip():
            raise Refused(
                f"an earlier run stopped while updating {m['name']}: "
                f"git switch {m['branch']}, then rerun"
            )
        else:
            mark.unlink()  # its branch is gone, and with it the half-applied scope
    if stuck := " ".join(unmerged(root)):
        raise Refused(f"unmerged paths: {stuck}; resolve them, git add, git commit, then rerun")
    if _dirty(root):
        raise Refused("working tree not clean; commit or stash your changes first")
    if hit := git("grep", "-lE", MARKER, "HEAD", cwd=root, ok=(1,)).split():
        files = ", ".join(h.split(":", 1)[1] for h in hit)
        raise Refused(f"conflict markers committed in {files}; resolve them and commit")


@contextlib.contextmanager
def undoing():
    """Ignore Ctrl-C while undoing: a second interrupt (a repeated Ctrl-C, or `uv run`
    forwarding the first) must not cut the undo short."""
    old = signal.signal(signal.SIGINT, signal.SIG_IGN)
    try:
        yield
    finally:
        signal.signal(signal.SIGINT, old)


@contextlib.contextmanager
def scope_run(root: Path, name: str, paths: tuple[str, ...]):
    """Run one scope: anything that stops it before it returns (an error, Ctrl-C) puts HEAD and
    the scope's paths back. The marker in .git covers what can't run that cleanup (a kill)."""
    head, mark = git("rev-parse", "HEAD", cwd=root).strip(), _marker_file(root)
    branch = git("branch", "--show-current", cwd=root).strip()
    mark.write_text(json.dumps({"branch": branch, "head": head, "name": name, "paths": paths}))
    try:
        yield
    except BaseException:
        with undoing():
            git("reset", "-q", "--hard", head, cwd=root)
            git("clean", "-fdq", "--", *paths, cwd=root)
            mark.unlink(missing_ok=True)
        raise
    mark.unlink(missing_ok=True)


@contextlib.contextmanager
def _copier():
    """Hide copier's stderr notices, and survive its temp-dir cleanup race (Errno 39), which
    fires after the copy or update is complete. The caller checks the result either way."""
    try:
        with contextlib.redirect_stderr(io.StringIO()):
            yield
    except OSError as e:
        tmp, left = Path(tempfile.gettempdir()), Path(e.filename or "/")
        inside = left.is_relative_to(tmp) and left != tmp
        top = left.relative_to(tmp).parts[0] if inside else ""
        if e.errno != errno.ENOTEMPTY or not top.startswith("copier."):
            raise
        shutil.rmtree(tmp / top, ignore_errors=True)


def _render(run, root: Path, scope: str, mirror: Path, tag: str, *src: str, **kw) -> None:
    """copier's run_copy or run_update into root/scope, at the commit the mirror verified.
    Settings(): a user copier settings file must not set `element`."""
    with _copier():
        dst, ref = root / scope, peel(mirror, tag)
        run(*src, dst, vcs_ref=ref, defaults=True, quiet=True, settings=Settings(), **kw)
    if answers(root, scope)["_commit"] != tag:
        raise Refused(f"{scope}: copier did not record {tag}")


def element(scope: str, framework: str = "") -> str:
    """The element a scope copies, or framework for the framework scope."""
    return framework if scope == "." else scope.removeprefix("stack/")


def copy(url: str, mirror: Path, tag: str, root: Path, scope: str) -> None:
    _render(run_copy, root, scope, mirror, tag, url, data={"element": element(scope)})


def _blob(root: Path, gitdir: Path, rev: str, path: str, mode: str, restore=False) -> str:
    """Stage entry "mode oid" for path at rev, its blob copied into root's object store;
    restore also writes it to the work tree with its mode."""
    data = read(gitdir, rev, [path])[path]
    if restore:
        write(root / path, data)
        (root / path).chmod(0o755 if mode == "100755" else 0o644)
    return f"{mode} {git('hash-object', '-w', '--stdin', cwd=root, stdin=data).strip()}"


def _stage(root: Path, path: str, *entries: str | None) -> None:
    """Record path as unmerged with stages 1 (base), 2 (ours), 3 (theirs), as copier does."""
    stages = [f"{e} {n}\t{path}" for n, e in enumerate(entries, 1) if e]
    lines = "\n".join([f"0 {'0' * 40}\t{path}", *stages])
    git("update-index", "--index-info", cwd=root, stdin=lines.encode())


def update(root: Path, mirror: Path, scope: str, new: str) -> list[str]:
    """copier update one scope to new, then repair what copier gets wrong: an adopter edit to a
    file upstream renamed is merged into the new name (F1), an adopter-edited file upstream
    deleted is kept as a modify/delete conflict (F2), and a file the adopter deleted that
    upstream changed or renamed is a delete/modify conflict (F3). F1 onto a file of the
    adopter's own, which copier already left as an add/add conflict, is an F2 instead, so
    neither conflict nests in the other. Returns unmerged paths; empty means ready to commit."""
    old = answers(root, scope)["_commit"]
    base, head = _trees(root, mirror, old, scope)
    after = tree(mirror, new, spec(mirror, new, scope))
    mine, moved = _mine(base, head), changes(mirror, old, new, scope)
    _render(run_update, root, scope, mirror, new, overwrite=True, conflict="inline")
    conflicted = set(unmerged(root))
    # _commit alone is no proof: every path upstream removed that the adopter never touched
    # must be gone.
    gone = [c[1] for c in moved if c[0][0] in "DR" and c[1] not in mine]
    if stale := [p for p in gone if (root / p).exists()]:
        raise Refused(f"{scope}: copier did not apply upstream deletions {stale}")
    for status, src, *dst in moved:
        path = dst[0] if dst else src
        if src not in mine or (src in head and (root / src).exists()):
            continue  # not the adopter's, or copier merged it in place
        b = _blob(root, mirror, old, src, base[src][0])
        if src not in head:  # F3: deleted by the adopter
            if path in after:
                theirs = _blob(root, mirror, new, path, after[path][0], restore=True)
                _stage(root, path, b, None, theirs)
        elif status == "D" or path in conflicted:  # F2
            ours = _blob(root, root, "HEAD", src, head[src][0], restore=True)
            _stage(root, src, b, ours, None)
        elif status[0] == "R" and (root / path).exists():  # F1
            ours = _blob(root, root, "HEAD", src, head[src][0])
            theirs = _blob(root, mirror, new, path, after[path][0])
            oids = [e.split()[1] for e in (ours, b, theirs)]
            # merge-file exits with the conflict count, at most 127; above that is an error
            # (a binary file), which refuses the scope.
            args = ("merge-file", *LABELS, "-p", "--object-id", *oids)
            merged = run(*args, cwd=root, ok=range(1, 128))
            (root / path).write_bytes(merged.stdout)
            if merged.returncode:
                _stage(root, path, b, ours, theirs)
    return unmerged(root)


def commit(root: Path, paths: tuple[str, ...], message: str) -> None:
    """Stage only the scope's paths, never with conflict markers."""
    git("add", "-A", "--", *paths, cwd=root)
    grep = ("grep", "-lE", "--cached", MARKER, "--", *paths)
    if hit := git(*grep, cwd=root, ok=(1,)).split():
        raise Refused(f"conflict markers in {', '.join(hit)}")
    git("commit", "-q", "-m", message, cwd=root)
