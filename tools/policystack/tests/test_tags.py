"""Release selection: ordering, prereleases, required stops, blocked tags read from the newest tag."""

import pytest
from conftest import init, release, sh

from policystack import catalog as cat


@pytest.fixture
def up(tmp_path):
    u = init(tmp_path)
    release(u, "v1.0.0")
    release(
        u,
        "v1.1.0",
        {
            "upgrades/a.yaml": "summary: hub annotation\nstop: true\n",
            "upgrades/b.yaml": "summary: rename\nops: []\n",
        },
    )
    release(u, "v1.2.0")
    release(
        u,
        "v1.2.1",
        {
            "upgrades/blocked.yaml": "- to: v1.2.0\n  message: odf waits forever\n  fixedIn: v1.2.1\n"
        },
    )
    release(u, "v1.10.0")
    release(u, "v2.0.0-rc.1")
    release(u, "v2.0.0")
    return u


def test_tags_sorted_semver_without_prereleases(up):
    assert cat.tags(up) == ["v1.0.0", "v1.1.0", "v1.2.0", "v1.2.1", "v1.10.0", "v2.0.0"]


def test_stops(up):
    assert cat.stops(up, cat.tags(up), {}) == {
        "v1.1.0": "hub annotation",
        "v1.10.0": "last v1 release before v2",
    }


def test_blocked_release_is_never_a_stop(up):
    """The stop moves to the newest unblocked release of the major, or to the next one shipping the step."""
    ts = cat.tags(up)
    assert cat.stops(up, ts, {"v1.10.0": "x"}) == {
        "v1.1.0": "hub annotation",
        "v1.2.1": "last v1 release before v2",
    }
    assert cat.stops(up, ts, {"v1.1.0": "x"}) == {
        "v1.2.0": "hub annotation",
        "v1.10.0": "last v1 release before v2",
    }


def test_blocked_last_of_major_does_not_strand_later_releases():
    """correctness/D4: v2.2.0 (last v2) blocked with fixedIn v3.0.0 must leave v3.0.0 reachable from v2.1.0."""
    ts, block = (
        ["v2.0.0", "v2.1.0", "v2.2.0", "v3.0.0"],
        {"v2.2.0": "breaks storage; use v3.0.0"},
    )
    stop = {"v2.1.0": "last v2 release before v3"}  # what stops() gives for this block
    assert cat.targets(ts, "v2.1.0", stop, block) == [
        ("v3.0.0", None, ""),
        ("v2.2.0", "blocked: breaks storage; use v3.0.0", ""),
    ]
    assert cat.targets(ts, "v2.0.0", stop, block)[0] == (
        "v3.0.0",
        "update to v2.1.0 first (required stop)",
        "",
    )


@pytest.mark.parametrize("tag", ["v2.0.0-rc.1", "v1.2.1-3-gabcdef0", "policy-library-4.1.3", None])
def test_version_refuses_non_release(tag):
    with pytest.raises(cat.Refused, match="not a vX.Y.Z release"):
        cat.version(tag)


def test_steps_exclude_blocked(up):
    assert list(cat.steps(up, "v1.2.1")) == ["a.yaml", "b.yaml"]
    assert cat.steps(up, "v1.0.0") == {}


def test_blocked_from_newest(up):
    assert cat.blocked(up, "v2.0.0") == {"v1.2.0": "odf waits forever; use v1.2.1"}
    assert cat.blocked(up, "v1.2.0") == {}  # the blocked tag itself can't know


def rows(up, current):
    ts = cat.tags(up)
    block = cat.blocked(up, ts[-1])
    return cat.targets(ts, current, cat.stops(up, ts, block), block)


def test_targets_hold_at_first_stop(up):
    assert rows(up, "v1.0.0") == [
        ("v2.0.0", "update to v1.1.0 first (required stop)", ""),
        (
            "v1.10.0",
            "update to v1.1.0 first (required stop)",
            "required stop: last v1 release before v2",
        ),
        ("v1.2.1", "update to v1.1.0 first (required stop)", ""),
        (
            "v1.2.0",
            "update to v1.1.0 first (required stop); blocked: odf waits forever; use v1.2.1",
            "",
        ),
        ("v1.1.0", None, "required stop: hub annotation"),
    ]


def test_targets_blocked_and_major(up):
    assert rows(up, "v1.1.0") == [
        ("v2.0.0", "update to v1.10.0 first (required stop)", ""),
        ("v1.10.0", None, "required stop: last v1 release before v2"),
        ("v1.2.1", None, ""),
        ("v1.2.0", "blocked: odf waits forever; use v1.2.1", ""),
    ]
    assert rows(up, "v1.10.0") == [("v2.0.0", None, "")]
    assert rows(up, "v2.0.0") == []


def test_install_lists_all_but_blocks(up):
    ts = cat.tags(up)
    got = cat.targets(ts, None, cat.stops(up, ts, {}), cat.blocked(up, ts[-1]))
    assert [r[0] for r in got] == ts[::-1]
    assert [r for r in got if r[1] or r[2]] == [
        ("v1.2.0", "blocked: odf waits forever; use v1.2.1", "")
    ]


def test_no_release_tags_names_the_url(tmp_path):
    sh(tmp_path, "git", "init", "-q", "--bare")
    sh(tmp_path, "git", "remote", "add", "origin", "https://git.example.invalid/x.git")
    with pytest.raises(
        cat.Refused,
        match=r"^https://git.example.invalid/x.git: no vX.Y.Z release tags$",
    ):
        cat.tags(tmp_path)
