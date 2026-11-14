"""git-level guards on synthetic repositories: scope paths, breaking subjects, tags copier would
record, adopter deletions, the clean-tree gate, the half-applied-scope marker and the tag cache."""

import json
import os
import signal

import pytest
from conftest import init, release, sh, write

from policystack import repo
from policystack.repo import Refused

COPIER = "_exclude:\n  - |\n    {% if not element %}\n    /stack\n    /docs\n    /README.md\n    {% endif %}\n"


@pytest.fixture
def up(tmp_path):
    """Upstream at v1.0.0: a framework file, docs, README.md, and element x with its answers template."""
    u = init(tmp_path / "up")
    release(
        u,
        "v1.0.0",
        {
            "copier.yml": COPIER,
            "tool.sh": "a\n",
            "docs/a.md": "a\n",
            "README.md": "a\n",
            "stack/x/values.yaml": "a\n",
            "stack/x/{{_copier_conf.answers_file}}.jinja": "a\n",
        },
    )
    return u


@pytest.mark.parametrize(
    "subject,want",
    [
        ("feat!: v2", True),
        ("feat(odf)!: drop x", True),
        ("feat(odf): breaking: drop x", True),
        ("breaking: loki drops the s3 mode", True),
        ("fix: breaking: CVE that can't be fixed compatibly", True),
        ("feat(odf): add x", False),
        ("fix: not breaking: really", False),
        ("docs: breaking changes page", False),
    ],
)
def test_breaking_subjects(subject, want):
    assert bool(repo.BREAKING.match(subject)) is want


def test_framework_and_element_paths_follow_copier_yml(up):
    """correctness/D9, D11: framework-only excludes don't count for the framework or for elements."""
    release(
        up,
        "v1.0.1",
        {
            "copier.yml": COPIER.replace("/docs\n", "/docs\n    /CHANGELOG.md\n"),
            "docs/a.md": "b\n",
            "README.md": "b\n",
        },
        "docs!: rewrite",
    )
    assert repo.changes(up, "v1.0.0", "v1.0.1", ".") == []
    assert repo.changes(up, "v1.0.0", "v1.0.1", "stack/x") == []
    assert not repo.breaking(up, "v1.0.0", "v1.0.1", ".")
    release(
        up, "v1.0.2", {"copier.yml": COPIER + "_min_copier_version: '9.19.0'\n", "tool.sh": "b\n"}
    )
    assert repo.changes(up, "v1.0.1", "v1.0.2", "stack/x") == [["M", "copier.yml"]]
    assert repo.changes(up, "v1.0.1", "v1.0.2", ".") == [["M", "tool.sh"]]


def test_peel_refuses_a_second_tag_copier_would_record(up):
    """safety/D4: copier records `git describe --tags`, which picks a-lib-4.1.3 over v1.0.0."""
    sha = sh(up, "git", "rev-parse", "v1.0.0").strip()
    sh(up, "git", "tag", "v1.0.0-rc.1", "v1.0.0")  # sorts after v1.0.0: describe still says v1.0.0
    assert repo.peel(up, "v1.0.0") == sha
    sh(up, "git", "tag", "a-lib-4.1.3", "v1.0.0")
    with pytest.raises(Refused, match="copier would record a-lib-4.1.3"):
        repo.peel(up, "v1.0.0")


@pytest.fixture
def adopter(up, tmp_path):
    """A clone standing in for a copy at v1.0.0 (same blobs), on main."""
    a = tmp_path / "a"
    sh(tmp_path, "git", "clone", "-q", str(up), str(a))
    return a


def test_modified_counts_deletions_not_answers_templates(up, adopter):
    """correctness/D8: a file the adopter deleted is theirs, like one they edited."""
    release(
        adopter, None, {"tool.sh": "mine\n", "stack/x/values.yaml": None, "README.md": "mine\n"}
    )
    assert repo.modified(adopter, up, "v1.0.0", ".") == {"tool.sh"}  # README.md is adopter-owned
    assert repo.modified(adopter, up, "v1.0.0", "stack/x") == {"stack/x/values.yaml"}


def test_clean_gate_ignores_show_untracked_config(adopter):
    """safety/D2: status.showUntrackedFiles=no must not hide untracked work."""
    sh(adopter, "git", "config", "status.showUntrackedFiles", "no")
    write(adopter, {"stack/x/wip.yaml": "x\n"})
    with pytest.raises(Refused, match="not clean"):
        repo.require_clean(adopter)


def test_committed_conflict_markers_refused(adopter):
    """safety/D1: a scope committed with copier's markers stops every later run, and commit refuses them."""
    write(adopter, {"tool.sh": "<" * 7 + " before updating\na\n=======\nb\n>>>>>>> after\n"})
    with pytest.raises(Refused, match="conflict markers in tool.sh"):
        repo.commit(adopter, (".",), "x")
    sh(adopter, "git", "commit", "-q", "-m", "by hand")
    with pytest.raises(Refused, match="conflict markers committed in tool.sh"):
        repo.require_clean(adopter)


@pytest.mark.parametrize(
    ("text", "refused"),
    [
        ("<" * 7 + " HEAD\na\n=======\nb\n>>>>>>> other\n", True),  # git merge's labels
        ("<" * 7 + "\n", False),  # no label: not a marker git writes
        (" " + "<" * 7 + " HEAD\n", False),
    ],
)
def test_any_committed_conflict_marker_refused(adopter, text, refused):
    """contract-docs/C1: the gate refuses a marker from any merge, not only copier's."""
    release(adopter, None, {"values.yaml": text})
    if refused:
        with pytest.raises(Refused, match="conflict markers committed in values.yaml"):
            repo.require_clean(adopter)
    else:
        repo.require_clean(adopter)


def test_scope_run_puts_the_scope_back_on_error(adopter):
    head = sh(adopter, "git", "rev-parse", "HEAD")
    with pytest.raises(KeyboardInterrupt), repo.scope_run(adopter, "x", ("stack/x",)):
        write(adopter, {"stack/x/values.yaml": "half\n", "stack/x/new.yaml": "new\n"})
        sh(adopter, "git", "add", "-A")
        raise KeyboardInterrupt
    assert sh(adopter, "git", "rev-parse", "HEAD") == head
    assert sh(adopter, "git", "status", "--porcelain", "--untracked-files=all") == ""
    assert not repo._marker_file(adopter).exists()
    repo.require_clean(adopter)


def test_marker_outliving_a_kill(adopter):
    """A run killed mid-scope leaves the marker: the rerun names the undo, then continues once it's done."""
    head = sh(adopter, "git", "rev-parse", "HEAD").strip()
    killed = repo.scope_run(adopter, "x", ("stack/x",))
    killed.__enter__()  # never exited, as if killed (the reference keeps GC from closing it)
    write(adopter, {"stack/x/values.yaml": "half\n", "stack/x/new.yaml": "new\n"})
    with pytest.raises(
        Refused,
        match=f"(?s)stopped while updating x.*git reset -q --hard {head} && git clean -fdq -- stack/x",
    ):
        repo.require_clean(adopter)
    sh(adopter, "sh", "-c", f"git reset -q --hard {head} && git clean -fdq -- stack/x")
    repo.require_clean(adopter)
    assert not repo._marker_file(adopter).exists()


def test_marker_on_another_branch(adopter):
    repo._marker_file(adopter).write_text(
        json.dumps(
            {
                "branch": "policystack/update-v9.0.0",
                "head": "0" * 40,
                "name": "odf",
                "paths": ["stack/odf"],
            }
        )
    )
    sh(adopter, "git", "branch", "policystack/update-v9.0.0")
    with pytest.raises(Refused, match="git switch policystack/update-v9.0.0"):
        repo.require_clean(adopter)
    sh(adopter, "git", "branch", "-D", "policystack/update-v9.0.0")
    repo.require_clean(adopter)  # its branch is gone: the marker is dropped
    assert not repo._marker_file(adopter).exists()


@pytest.mark.parametrize("prune", [False, True])
@pytest.mark.parametrize("delete", [False, True])
def test_tag_cache_refuses_a_moved_tag(up, tmp_path, monkeypatch, prune, delete):
    """safety/D3: a tag moved, or deleted and made again elsewhere, is refused, also under a user
    fetch.prune and fetch.pruneTags, which would otherwise drop the cached tag first."""
    monkeypatch.setenv("XDG_CACHE_HOME", str(tmp_path / "cache"))
    if prune:
        for i, kv in enumerate((("fetch.prune", "true"), ("fetch.pruneTags", "true"))):
            monkeypatch.setenv(f"GIT_CONFIG_KEY_{i}", kv[0])
            monkeypatch.setenv(f"GIT_CONFIG_VALUE_{i}", kv[1])
        monkeypatch.setenv("GIT_CONFIG_COUNT", "2")
    url = f"file://{up}"
    good = sh(repo.mirror(url), "git", "rev-parse", "v1.0.0").strip()
    if delete:
        sh(up, "git", "tag", "-d", "v1.0.0")
        assert sh(repo.mirror(url), "git", "rev-parse", "v1.0.0").strip() == good
    release(up, None, {"tool.sh": "evil\n"}, "evil")
    sh(up, "git", "tag", "-f", "v1.0.0")
    with pytest.raises(
        Refused, match=r"(?s)upstream moved a released tag.*delete .*\.git and rerun"
    ):
        repo.mirror(url)


def test_undo_survives_a_second_interrupt(adopter, monkeypatch):
    """safety/D1: `uv run` forwards the Ctrl-C the process group already got; that second SIGINT
    landed mid-undo and left the scope half put back."""

    def git(*args, **kw):
        if args[0] == "reset":
            os.kill(os.getpid(), signal.SIGINT)
        return real(*args, **kw)

    real = repo.git
    with pytest.raises(KeyboardInterrupt), repo.scope_run(adopter, "x", ("stack/x",)):
        write(adopter, {"stack/x/new.yaml": "new\n"})
        monkeypatch.setattr(repo, "git", git)
        raise KeyboardInterrupt
    assert sh(adopter, "git", "status", "--porcelain", "--untracked-files=all") == ""
    assert not repo._marker_file(adopter).exists()
