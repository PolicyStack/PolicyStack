"""install, add and update end to end, without a terminal, against a synthetic upstream that uses
this checkout's copier.yml and answers template: copier really renders every scope."""

import contextlib
import io
import shutil
import stat
from pathlib import Path

import pytest
from conftest import ROOT, init, release, sh, write

from policystack import cli, repo

ANSWERS = (ROOT / "{{_copier_conf.answers_file}}.jinja").read_text()
CONDITIONS = "stack/odf/converters/health/conditions.yaml"
STATUS = "stack/odf/converters/health/status.yaml"
PDB = "stack/odf/converters/config/pdb-alert.yaml"
AVAILABLE = "stack/odf/converters/ready/available.yaml"
# Long enough that git sees the v1.2.1 rename with an edit as a rename.
COND = "".join(f"condition{i}: true\n" for i in range(8))
STEP = """summary: user-monitoring retention becomes retentionTime
files: ["values/**/*.yaml", "fleet/**/*.yaml"]
ops:
  - rename: stack.userMonitoring.config.prometheus.retention
    to: retentionTime
"""


def element(name, key=None, deps=(), optional=(), config=""):
    """stack/<name>: a chart whose policy p depends on deps and whose disabled policy q depends
    on optional."""

    def pol(names):
        return "".join(f"\n          - name: ready\n            element: {d}" for d in names)

    values = (
        f"stack:\n  {key or name}:\n    enabled: false\n{config}    policies:\n"
        f"      p:\n        dependencies:{pol(deps) or ' []'}\n"
        f"      q:\n        enabled: false\n        dependencies:{pol(optional) or ' []'}\n"
    )
    chart = f"apiVersion: v2\nname: {name}\ndescription: {name} element\nversion: 0.1.0\n"
    return {
        f"stack/{name}/Chart.yaml": chart,
        f"stack/{name}/values.yaml": values,
        f"stack/{name}/README.md": f"# {name}\n",
        f"stack/{name}/{{{{_copier_conf.answers_file}}}}.jinja": ANSWERS,
    }


UM = "    config:\n      prometheus:\n        retention: 24h\n"


@pytest.fixture(scope="module")
def up(tmp_path_factory):
    """Upstream: v1.0.0; v1.1.0 (an upgrade step, a required stop, newel); v1.2.0 (blocked by
    v1.2.1); v1.2.1 (odf renames and deletes converters, the hub chart changes); v2.0.0-rc.1 and
    v2.0.0 (a major that drops newel)."""
    u = init(tmp_path_factory.mktemp("up") / "up.git")  # copier takes file:// URLs ending .git only
    v1 = {
        "copier.yml": (ROOT / "copier.yml").read_text(),
        "{{_copier_conf.answers_file}}.jinja": ANSWERS,
        "install/values.yaml": "hub: {}\n",
        "tools/create-element.sh": "#!/bin/sh\n",
        "README.md": "upstream\n",
        "docs/index.md": "docs\n",
        "values/clusters/prod.yaml": (
            "stack:\n  machineHealth:\n    enabled: true\n  userMonitoring:\n    enabled: true\n"
            "    config:\n      prometheus:\n        # two weeks\n        retention: 15d\n"
        ),
        CONDITIONS: COND,
        PDB: "alert: pdb\n",
        AVAILABLE: "available: true\n",
        "stack/odf/logo.bin": b"\x00\x01binary\x00",
    }
    for args in (
        ("acm", None, (), ("odf",)),
        ("gitops",),
        ("upgrade",),
        ("storage-nodes", "storageNodes"),
        ("odf", None, ("storage-nodes",)),
        ("loki", None, ("odf",), ("coo",)),
        ("machine-health", "machineHealth"),
    ):
        v1 |= element(*args)
    v1 |= element("user-monitoring", "userMonitoring", config=UM)
    write(u, v1)
    (u / "tools/create-element.sh").chmod(0o755)
    release(u, "v1.0.0")
    release(
        u,
        None,
        {
            "upgrades/2026-11-10-um.yaml": STEP,
            "stack/user-monitoring/values.yaml": element(
                "user-monitoring",
                "userMonitoring",
                config=UM.replace("retention:", "retentionTime:"),
            )["stack/user-monitoring/values.yaml"],
        },
        "feat(user-monitoring)!: retention becomes retentionTime",
    )
    release(
        u,
        "v1.1.0",
        {
            "upgrades/2026-11-11-hub.yaml": "summary: hub annotation\nstop: true\nmanual: wait\n",
            "stack/storage-nodes/README.md": "# storage-nodes\nupstream v1.1\n",
        }
        | element("newel"),
    )
    release(u, "v1.2.0", {"stack/odf/README.md": "# odf\nv1.2.0\n"})
    sh(u, "git", "mv", CONDITIONS, STATUS)
    sh(u, "git", "mv", "stack/odf/logo.bin", "stack/odf/icon.bin")
    release(
        u,
        "v1.2.1",
        {
            STATUS: COND + "# upstream v1.2.1\n",
            PDB: None,
            "install/values.yaml": "hub: {}\n# v1.2.1\n",
            "stack/newel/README.md": "# newel\nv1.2.1\n",
            "upgrades/blocked.yaml": "- to: v1.2.0\n  message: odf waits forever\n  fixedIn: v1.2.1\n",
        },
    )
    sh(u, "git", "rm", "-rq", "stack/newel")
    release(u, "v2.0.0-rc.1", {"install/values.yaml": "hub: {}\n# v2\n"}, "feat!: v2 drops newel")
    sh(u, "git", "tag", "v2.0.0")
    return f"file://{u}"


def ps(capsys, *argv) -> tuple[int, str]:
    rc = cli.main(list(argv))
    out = capsys.readouterr()
    return rc, out.out + out.err


def commit_of(root: Path, scope: str = ".") -> str:
    return repo.answers(root, scope)["_commit"]


def log(root: Path, rev: str = "main..") -> list[str]:
    return sh(root, "git", "log", "--format=%s", rev).splitlines()


def clone(src: Path, dst: Path, monkeypatch) -> Path:
    shutil.copytree(src, dst, symlinks=True)
    monkeypatch.chdir(dst)
    return dst


@pytest.fixture(scope="module")
def installed(up, tmp_path_factory):
    """One install at v1.0.0 with odf, loki and an example, and what it printed; tests take a
    copy."""
    dst = tmp_path_factory.mktemp("installed") / "ad"
    argv = ["install", "--src", up, "--to", "v1.0.0", "--elements", "odf, loki"]
    out = io.TextIOWrapper(io.BytesIO(), write_through=True)  # main() reconfigures stdout
    with contextlib.redirect_stdout(out):
        assert cli.main([*argv, "--examples", "values/clusters/prod.yaml", str(dst)]) == 0
    return dst, out.buffer.getvalue().decode()


@pytest.fixture
def ad(installed, tmp_path, monkeypatch):
    return clone(installed[0], tmp_path / "ad", monkeypatch)


def resolve(ad: Path, element: str, tag: str, *rm: str) -> None:
    """Finish a stopped scope as the launcher prints: git rm what's not needed, add, commit."""
    if rm:
        sh(ad, "git", "rm", "-q", *rm)
    sh(ad, "git", "add", "-A", "--", f"stack/{element}")
    sh(ad, "git", "commit", "-qm", f"chore(policystack): update {element} to {tag}")


def edits(ad: Path) -> None:
    """Adopter edits the v1.0.0 -> v1.2.1 flow conflicts on, and a values layer for the step."""
    readme = (ad / "stack/storage-nodes/README.md").read_text() + "adopter note\n"
    release(
        ad,
        None,
        {
            "stack/storage-nodes/README.md": readme,
            CONDITIONS: "# adopter: tuned\n" + COND,
            PDB: "# adopter: keep\nalert: pdb\n",
            "fleet/prod.yaml": "revision: main\n",
            "values/common.yaml": (
                "# adopter layer\nstack:\n\n  userMonitoring:\n    config:\n      prometheus:\n"
                "        # a week\n        retention: 7d\n"
            ),
        },
        "adopter edits",
    )


def test_install(installed, up):
    ad, out = installed
    for line in (
        "copying framework (1/9)",
        "+ storage-nodes needed by odf",
        "+ machine-health enabled by values/clusters/prod.yaml",
        (
            "not added, named only by policies that ship disabled; POLICY011 fails once a "
            "cluster enables one: loki -> coo"
        ),
    ):
        assert line in out
    assert sorted(p.name for p in (ad / "stack").iterdir()) == [
        "acm",
        "gitops",
        "loki",
        "machine-health",
        "odf",
        "storage-nodes",
        "upgrade",
        "user-monitoring",
    ]
    scopes = [".", *(f"stack/{p.name}" for p in (ad / "stack").iterdir())]
    assert {(commit_of(ad, s), repo.answers(ad, s)["_src_path"]) for s in scopes} == {
        ("v1.0.0", up)
    }
    assert [str(p.relative_to(ad)) for p in (ad / "values").rglob("*.yaml")] == [
        "values/clusters/prod.yaml"
    ]
    assert not (ad / "docs").exists() and not (ad / "README.md").exists()
    assert (ad / "tools/create-element.sh").stat().st_mode & stat.S_IXUSR
    assert log(ad, "HEAD") == ["chore: PolicyStack v1.0.0"]
    assert sh(ad, "git", "remote") == "" and sh(ad, "git", "status", "--porcelain") == ""


@pytest.mark.parametrize(
    ("argv", "err"),
    [
        (("--to", "v1.2.0"), "blocked: odf waits forever; use v1.2.1"),
        (("--to", "v2.0.0-rc.1"), "copier would record v2.0.0 for this commit"),  # one commit
        (("--to", "latest"), "(you can move to: v2.0.0, v1.2.1, v1.1.0, v1.0.0)"),
        (("--elements", "odf, odff"), "unknown elements: odff (did you mean odf?)"),
        (("--examples", "values/x.yaml"), "unknown example files: values/x.yaml"),
    ],
)
def test_install_refusals(up, tmp_path, capsys, argv, err):
    args = {"--to": "v1.0.0", "--elements": ""} | dict(zip(argv[::2], argv[1::2]))
    rc, out = ps(capsys, "install", "--src", up, *sum(args.items(), ()), str(tmp_path / "x"))
    assert rc == 1 and err in out and not (tmp_path / "x").exists()


def test_install_default_is_newest(up, tmp_path):
    assert cli.main(["install", "--src", up, "--elements", "", str(tmp_path / "x")]) == 0
    assert commit_of(tmp_path / "x") == "v2.0.0"


def test_install_and_update_a_prerelease(up, tmp_path, monkeypatch, capsys):
    """--to names a prerelease without --pre, and update moves it on to the release. As
    release-please cuts them, the candidate and the release are separate commits."""
    u = tmp_path / "up.git"
    shutil.copytree(up.removeprefix("file://"), u, symlinks=True)
    release(u, "v2.0.1-rc.1", {"stack/acm/rc.txt": "rc\n"})
    release(u, "v2.0.1", {"stack/acm/rc.txt": "final\n"})
    ad, src = tmp_path / "x", f"file://{u}"
    argv = ["install", "--src", src, "--to", "v2.0.1-rc.1", "--elements", "", str(ad)]
    assert cli.main(argv) == 0 and commit_of(ad) == "v2.0.1-rc.1"
    monkeypatch.chdir(ad)
    assert ps(capsys, "update", "--to", "v2.0.1")[0] == 0
    assert commit_of(ad) == commit_of(ad, "stack/acm") == "v2.0.1"


def test_install_interrupted_removes_everything(up, tmp_path, monkeypatch, capsys):
    monkeypatch.chdir(tmp_path)
    real, calls = repo.copy, []

    def copy(*args):
        calls.append(args)
        if len(calls) == 2:
            raise KeyboardInterrupt
        real(*args)

    monkeypatch.setattr(repo, "copy", copy)
    rc, out = ps(capsys, "install", "--src", up, "--to", "v1.0.0", "--elements", "", "x")
    assert rc == 130 and "cancelled" in out and not (tmp_path / "x").exists()


def test_add(ad, capsys):
    rc, out = ps(capsys, "add", "newel")
    assert rc == 1 and "newel first ships in v1.1.0; update the framework first: " in out
    assert "mise run update --to v1.1.0" in out
    assert ps(capsys, "add", "odf")[1].endswith("already in stack/: odf\n")
    assert ps(capsys, "add")[0] == 1  # no names and no terminal
    write(ad, {"stack/acm/README.md": None})
    sh(ad, "git", "rm", "-rq", "stack/machine-health")
    sh(ad, "git", "commit", "-qm", "drop machine-health")
    rc, out = ps(capsys, "add", "machine-health")
    assert rc == 1 and "not clean" in out
    sh(ad, "git", "checkout", "--", "stack/acm/README.md")
    assert ps(capsys, "add", "machine-health")[0] == 0
    assert commit_of(ad, "stack/machine-health") == "v1.0.0"
    assert log(ad, "-1") == ["feat: add machine-health (v1.0.0)"]


@pytest.mark.parametrize(
    ("argv", "err"),
    [
        (("storage-nodes",), "these must move in this update too: user-monitoring (upgrade step "),
        (("acm",), "nothing to update in acm: unchanged upstream up to v1.1.0"),
        (("nope",), "unknown elements (not installed): nope"),
    ],
)
def test_update_subset_refusals(ad, capsys, argv, err):
    rc, out = ps(capsys, "update", "--to", "v1.1.0", *argv)
    assert rc == 1 and err in out and log(ad, "main..HEAD") == []


def test_update_target_refusals(ad, capsys):
    assert "pass --to <release>" in ps(capsys, "update")[1]
    assert "update to v1.1.0 first (required stop)" in ps(capsys, "update", "--to", "v1.2.1")[1]
    assert "blocked: odf waits forever" in ps(capsys, "update", "--to", "v1.2.0")[1]
    write(ad, {"values/new.yaml": "x: 1\n"})
    assert "not clean" in ps(capsys, "update", "--to", "v1.1.0")[1]
    assert sh(ad, "git", "branch", "--show-current") == "main\n"


def test_update_conflict_stop_and_resume(ad, capsys):
    edits(ad)
    before = (ad / "values/clusters/prod.yaml").read_text()
    rc, out = ps(capsys, "update", "--to", "v1.1.0")
    assert (
        rc == 1 and "stopped: conflicts in storage-nodes:\n  stack/storage-nodes/README.md" in out
    )
    for line in (
        "  upgrade step 2026-11-11-hub.yaml: hub annotation\n    before the next release: wait",
        "  upgrade steps rewrite: values/clusters/prod.yaml, values/common.yaml",
        "storage-nodes        1 file changed, you modified 1, 1 overlap -> may conflict",
        "user-monitoring      1 file changed, BREAKING",
        "  new upstream (mise run ps -- add): newel",
        (
            "  git add -A -- stack/storage-nodes && git commit -m "
            "'chore(policystack): update storage-nodes to v1.1.0'"
        ),
        "  mise run update --to v1.1.0\n",
    ):
        assert line in out
    assert sh(ad, "git", "branch", "--show-current") == "policystack/update-v1.1.0\n"
    assert log(ad) == [
        "chore(policystack): update framework to v1.1.0",
        "chore(policystack): upgrade steps for v1.1.0",
    ]
    assert (ad / "values/clusters/prod.yaml").read_text() == before.replace(
        "retention:", "retentionTime:"
    )
    assert (
        "        # a week\n        retentionTime: 7d\n" in (ad / "values/common.yaml").read_text()
    )
    assert (ad / "fleet/prod.yaml").read_text() == "revision: main\n"
    rc, out = ps(capsys, "update", "--to", "v1.1.0")
    assert rc == 1 and "unmerged paths: stack/storage-nodes/README.md" in out
    write(ad, {"stack/storage-nodes/README.md": "# storage-nodes\nupstream v1.1\nadopter note\n"})
    resolve(ad, "storage-nodes", "v1.1.0")
    rc, out = ps(capsys, "update", "--to", "v1.1.0")
    assert rc == 0 and "[x] user-monitoring" in out and "framework:" not in out
    assert log(ad, "-1") == ["chore(policystack): update user-monitoring to v1.1.0"]
    assert [commit_of(ad, s) for s in (".", "stack/odf", "stack/user-monitoring")] == [
        "v1.1.0",
        "v1.0.0",
        "v1.1.0",
    ]
    rc, out = ps(capsys, "update", "--to", "v1.1.0")
    assert rc == 0 and "up to date" in out
    sh(ad, "git", "switch", "-q", "main")
    rc, out = ps(capsys, "update", "--to", "v1.1.0")
    assert rc == 1 and "policystack/update-v1.1.0 exists. To finish it: git switch" in out


@pytest.fixture(scope="module")
def v110(installed, tmp_path_factory):
    """The install with the edits, at v1.1.0 on main, and newel added. storage-nodes stops on
    its README, which the adopter deleted and upstream changed (F3)."""
    with pytest.MonkeyPatch.context() as mp:
        ad = clone(installed[0], tmp_path_factory.mktemp("v110") / "ad", mp)
        edits(ad)
        sh(ad, "git", "rm", "-q", "stack/storage-nodes/README.md")
        sh(ad, "git", "commit", "-qm", "drop the README")
        assert cli.main(["update", "--to", "v1.1.0"]) == 1
        assert sh(ad, "git", "status", "--porcelain", "stack/storage-nodes/README.md") == (
            "DU stack/storage-nodes/README.md\n"
        )
        resolve(ad, "storage-nodes", "v1.1.0", "stack/storage-nodes/README.md")
        assert cli.main(["update", "--to", "v1.1.0"]) == 0
        sh(ad, "git", "switch", "-q", "main")
        sh(ad, "git", "merge", "-q", "--ff-only", "policystack/update-v1.1.0")
        assert cli.main(["add", "newel"]) == 0
    return ad


@pytest.fixture
def at_v110(v110, tmp_path, monkeypatch):
    return clone(v110, tmp_path / "ad", monkeypatch)


def test_update_f1_f2_hub(at_v110, capsys):
    """F1: an adopter edit to a file upstream renamed lands in the new name, merged with
    upstream's edit. F2: an edited file upstream deleted is kept, 'deleted by them'."""
    ad = at_v110
    rc, out = ps(capsys, "update", "--to", "v1.2.1", "odf")
    assert rc == 1 and f"stopped: conflicts in odf:\n  {PDB}" in out
    assert "hub chart changed: before merging, run on every hub from this branch:" in out
    assert "[ ] newel" in out
    assert sh(ad, "git", "status", "--porcelain", PDB) == f"UD {PDB}\n"
    assert (ad / PDB).read_text().startswith("# adopter: keep")
    assert not (ad / CONDITIONS).exists()
    assert (ad / STATUS).read_text() == "# adopter: tuned\n" + COND + "# upstream v1.2.1\n"
    resolve(ad, "odf", "v1.2.1", PDB)
    rc, out = ps(capsys, "update", "--to", "v1.2.1")
    assert rc == 0 and "[x] newel" in out and "hub chart changed" in out
    assert commit_of(ad, "stack/newel") == "v1.2.1"


def test_update_f1_onto_a_file_of_the_adopters(at_v110, capsys):
    """The adopter edited the file upstream renames and has their own at the new name: two flat
    conflicts, the add/add at the new name and the edited old file kept as 'deleted by them'."""
    ad = at_v110
    release(ad, None, {STATUS: "# adopter's own status\n"})
    assert ps(capsys, "update", "--to", "v1.2.1", "odf")[0] == 1
    assert sh(ad, "git", "status", "--porcelain", CONDITIONS, STATUS) == (
        f"UD {CONDITIONS}\nUU {STATUS}\n"
    )
    assert (ad / CONDITIONS).read_text() == "# adopter: tuned\n" + COND
    text = (ad / STATUS).read_text()
    assert text.count("<" * 7) == 1 and "# adopter's own status" in text
    assert "# upstream v1.2.1" in text


def test_update_f3_renamed(at_v110, capsys):
    """F3: a file the adopter deleted that upstream renamed is 'deleted by us' at the new name."""
    ad = at_v110
    sh(ad, "git", "rm", "-q", CONDITIONS)
    release(ad, None, msg="drop conditions")
    assert ps(capsys, "update", "--to", "v1.2.1", "odf")[0] == 1
    assert sh(ad, "git", "status", "--porcelain", STATUS) == f"DU {STATUS}\n"


def test_update_binary_rename_refused(at_v110, capsys):
    """An edited binary file upstream renamed can't be merged: the scope is refused and put
    back, never written as an empty file."""
    ad = at_v110
    release(ad, None, {"stack/odf/logo.bin": b"\x00\x02mine\x00"}, "edit the logo")
    rc, out = ps(capsys, "update", "--to", "v1.2.1", "odf")
    assert rc == 1 and "git merge-file" in out and "binary" in out
    assert sh(ad, "git", "status", "--porcelain") == "" and commit_of(ad, "stack/odf") == "v1.0.0"
    assert log(ad) == ["chore(policystack): update framework to v1.2.1"]


def test_update_refused_before_any_commit_drops_the_branch(at_v110, capsys, monkeypatch):
    """A refusal before the first commit leaves the adopter where they started."""

    def refuse(*_):
        raise repo.Refused("framework: copier did not record v1.2.1")

    monkeypatch.setattr(repo, "update", refuse)
    rc, out = ps(capsys, "update", "--to", "v1.2.1", "odf")
    assert rc == 1 and "did not record" in out
    assert sh(at_v110, "git", "branch", "--show-current") != "policystack/update-v1.2.1\n"
    assert sh(at_v110, "git", "branch", "--list", "policystack/update-v1.2.1") == ""


def test_update_major(up, tmp_path, monkeypatch, capsys):
    """A major moves every element and refuses while an element it removed is tracked."""
    ad = tmp_path / "ad"
    argv = ["install", "--src", up, "--to", "v1.2.1", "--elements", "newel", str(ad)]
    assert cli.main(argv) == 0
    monkeypatch.chdir(ad)
    rc, out = ps(capsys, "update", "--to", "v2.0.0", "gitops")
    assert rc == 1 and "removed upstream by v2.0.0: newel" in out
    sh(ad, "git", "rm", "-q", "stack/newel/.copier-answers.yml")
    sh(ad, "git", "commit", "-qm", "keep newel as mine")
    rc, out = ps(capsys, "update", "--to", "v2.0.0", "gitops")
    assert rc == 1 and "acm (a major moves every element)" in out
    rc, out = ps(capsys, "update", "--to", "v2.0.0")
    assert rc == 0 and "major release: follow docs/guides/upgrading.md#majors" in out
    assert "untracked (yours): newel" in out
    scopes = [".", "stack/acm", "stack/gitops", "stack/upgrade"]
    assert {commit_of(ad, s) for s in scopes} == {"v2.0.0"} and len(log(ad)) == len(scopes)
