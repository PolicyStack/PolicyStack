"""Shared plumbing: every test runs git with an isolated config, and builds repositories with sh,
write and release."""

import subprocess
from pathlib import Path

import pytest

ROOT = Path(__file__).resolve().parents[3]  # tests/ is never copied: this is upstream


@pytest.fixture(scope="session", autouse=True)
def git_config(tmp_path_factory):
    """An identity and no user or system git settings, so a developer's config (fetch.prune,
    url.insteadOf, signing) can't change a result. The launcher's mirrors cache next to it."""
    home = tmp_path_factory.mktemp("home")
    (home / "gitconfig").write_text("[user]\n\tname = t\n\temail = t@x\n")
    with pytest.MonkeyPatch.context() as mp:
        mp.setenv("GIT_CONFIG_GLOBAL", str(home / "gitconfig"))
        mp.setenv("GIT_CONFIG_NOSYSTEM", "1")
        mp.setenv("XDG_CACHE_HOME", str(home / "cache"))
        yield


def sh(cwd, *cmd) -> str:
    return subprocess.run(cmd, cwd=cwd, check=True, capture_output=True, text=True).stdout


def write(root: Path, files: dict[str, str | bytes | None]) -> None:
    """Write files under root; None deletes one."""
    for name, data in files.items():
        path = root / name
        if data is None:
            path.unlink()
            continue
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(data if isinstance(data, bytes) else data.encode())


def release(repo: Path, tag: str | None, files=None, msg: str | None = None) -> None:
    """Commit files (as write takes them) in repo, and tag the commit unless tag is None."""
    write(repo, files or {})
    sh(repo, "git", "add", "-A")
    sh(repo, "git", "commit", "-q", "--allow-empty", "-m", msg or tag or "x")
    if tag:
        sh(repo, "git", "tag", tag)


def init(path: Path) -> Path:
    path.mkdir(parents=True, exist_ok=True)
    sh(path, "git", "init", "-q", "-b", "main")
    return path
