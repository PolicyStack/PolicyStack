"""CLI refusals that need no upstream, and the picker's name-only filter."""

from types import SimpleNamespace

import pytest
from conftest import init, release
from questionary.prompts.common import InquirerControl

from policystack import cli


@pytest.fixture
def repo_dir(tmp_path, monkeypatch):
    release(init(tmp_path), None)
    monkeypatch.chdir(tmp_path)
    return tmp_path


def run(capsys, *argv):
    rc = cli.main(list(argv))
    return rc, capsys.readouterr().err


def el(name, hard=(), optional=(), description=""):
    return cli.cat.Element(name, description, "preview", hard, optional)


def test_filter_matches_names_only(monkeypatch):
    """ux/D3: 'odf' must not land on loki, whose description says 'needs odf'."""
    elements = {
        "loki": el("loki", ("odf",), description="Loki log storage"),
        "odf": el("odf", ("storage-nodes",), description="OpenShift Data Foundation"),
    }
    seen = []

    def checkbox(_, choices, **__):
        seen.extend(choices)
        return SimpleNamespace(unsafe_ask=list)

    monkeypatch.setattr(cli.questionary, "checkbox", checkbox)
    cli._checkbox("Elements", cli._element_rows(elements, elements))
    ic = InquirerControl(seen)
    ic.search_filter = "odf"
    assert [c.value for c in ic.filtered_choices] == ["odf"]
    assert [c.description for c in seen] == [
        "needs odf. Loki log storage",
        "needs storage-nodes. OpenShift Data Foundation",
    ]


@pytest.mark.parametrize(
    ("argv", "err"),
    [
        # ux/D10
        (
            ("update", "--to", "v1.1.0"),
            "not a PolicyStack repository (no .copier-answers.yml at ",
        ),
        (("add", "odf"), "not a PolicyStack repository"),
    ],
)
def test_not_a_policystack_repo(repo_dir, capsys, argv, err):
    rc, out = run(capsys, *argv)
    assert rc == 1 and err in out and "Traceback" not in out


def test_empty_to_is_missing_not_a_prompt(repo_dir, capsys):
    """ux/D11: --to '' without a terminal names the flag instead of prompt_toolkit's EOFError."""
    release(
        repo_dir, None, {".copier-answers.yml": "_commit: v1.0.0\n_src_path: file:///nonexistent\n"}
    )
    assert run(capsys, "update", "--to", "") == (
        1,
        "policystack: no terminal (stdin and stdout must both be one): pass --to <release>\n",
    )
    rc, err = run(capsys, "add")  # ux/D15: add takes names, not --elements
    assert rc == 1 and "name the elements: mise run ps -- add <element>..." in err


def test_install_inside_a_work_tree_refused(repo_dir, capsys):
    """ux/D22: a nested repository would block the outer one's next update."""
    rc, err = run(capsys, "install", "--elements", "", "nested")
    assert rc == 1 and "inside the git work tree" in err
    assert not (repo_dir / "nested").exists()


def test_csv_strips():
    assert cli._csv("odf, loki ,,") == ["odf", "loki"]  # ux/D18
    assert cli._csv("") == []


def test_unknown_suggests():
    with pytest.raises(cli.Refused, match=r"unknown elements: odff \(did you mean odf\?\)"):
        cli._unknown(["odff"], ["odf", "loki"], "elements")


def test_tag_error_lists_releases():
    rows = [
        ("v1.2.0", "update to v1.1.0 first (required stop)", ""),
        ("v1.1.0", None, ""),
    ]
    with pytest.raises(
        cli.Refused,
        match=r"latest: not a release you can move to \(you can move to: v1.1.0\)",
    ):
        cli._check_tag("latest", rows)


def test_summary_names_policy011_and_skips_installed(capsys):
    """ux/D2, D14: deps of disabled policies are named with when POLICY011 fails, and installed
    ones aren't listed."""
    elements = {
        "ambient-mesh": el("ambient-mesh", optional=("odf", "tempo")),
        "odf": el("odf"),
        "tempo": el("tempo"),
    }
    cli._summary(elements, {"ambient-mesh"}, {}, {}, have={"odf"})
    assert capsys.readouterr().out.splitlines()[-1] == (
        "  not added, named only by policies that ship disabled; POLICY011 fails once a "
        "cluster enables one: ambient-mesh -> tempo"
    )


def test_copier_user_message_is_a_refusal(monkeypatch, capsys):
    """safety/D11: copier's own refusals (a downgrade) print as one line, not a traceback."""

    def downgrade(_):
        raise cli.UserMessageError("You are downgrading from 1.2.2 to 1.2.0.")

    monkeypatch.setattr(cli, "update", downgrade)
    assert run(capsys, "update", "--to", "v1.2.0") == (
        1,
        "policystack: You are downgrading from 1.2.2 to 1.2.0.\n",
    )
