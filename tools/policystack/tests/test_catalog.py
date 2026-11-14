"""Element dependencies and example files: generic checks against this checkout at HEAD, and the
logic on a synthetic catalog, so editing an element never fails these tests."""

import re

import pytest
from conftest import ROOT

from policystack import catalog as cat


@pytest.fixture(scope="module")
def elements():
    return cat.elements(ROOT, "HEAD")


def test_every_element_read(elements):
    assert sorted(elements) == sorted(p.parent.name for p in ROOT.glob("stack/*/Chart.yaml"))
    assert {e.stability for e in elements.values()} <= {"stable", "preview"}
    assert all(e.description for e in elements.values())


def test_edges_match_grep(elements):
    """Every `element:` line in stack/*/values.yaml is one dependency edge, and nothing else is."""
    line = re.compile(r"^\s*-?\s*element:\s*([\w-]+)", re.MULTILINE)
    grep = {
        (f.parent.name, m[1])
        for f in ROOT.glob("stack/*/values.yaml")
        for m in line.finditer(f.read_text())
        if m[1] != f.parent.name
    }
    assert {(n, d) for n, e in elements.items() for d in e.hard + e.optional} == grep
    assert grep  # the checkout has cross-element dependencies


def test_hard_and_optional():
    """A dependency named by an enabled policy is hard, even when a disabled one also names it;
    the element's own name and entries without element: are not dependencies."""
    policies = {
        "a": {"dependencies": [{"element": "odf"}, {"element": "loki"}, {"name": "x"}]},
        "b": {"enabled": False, "dependencies": [{"element": "odf"}, {"element": "coo"}]},
        "c": None,
    }
    values = {"stack": {"loki": {"policies": policies}, "other": {}}}
    assert cat.deps("loki", values) == (("odf",), ("coo",))
    assert cat.deps("loki", None) == ((), ())


@pytest.mark.parametrize(
    ("enabled", "hard"),
    [
        (None, True),
        (True, True),
        (0, True),
        ("no", True),
        (False, False),
        ("false", False),
    ],
)
def test_enabled_as_policy_library_reads_it(enabled, hard):
    """policy-library.enabled: anything but false or "false" is on."""
    policy = {"dependencies": [{"element": "odf"}]}
    if enabled is not None:
        policy["enabled"] = enabled
    values = {"stack": {"x": {"policies": {"p": policy}}}}
    assert cat.deps("x", values) == ((("odf",), ()) if hard else ((), ("odf",)))


def test_closure():
    """Hard dependencies, transitively, each named by the element that needs it; optional ones
    are never added."""

    def el(name, hard=(), optional=()):
        return cat.Element(name, "", "preview", hard, optional)

    catalog = {
        "loki": el("loki", ("odf",), ("coo",)),
        "odf": el("odf", ("storage-nodes",)),
        "storage-nodes": el("storage-nodes"),
        "coo": el("coo"),
        "acm": el("acm", optional=("odf",)),
    }
    assert cat.closure(catalog, {"loki"}) == (
        {"loki", "odf", "storage-nodes"},
        {"odf": "loki", "storage-nodes": "odf"},
    )
    assert cat.closure(catalog, {"acm", "odf"}) == (
        {"acm", "odf", "storage-nodes"},
        {"storage-nodes": "odf"},
    )


def test_examples(elements):
    ex = cat.examples(ROOT, "HEAD", elements)
    assert set(ex) == {str(p.relative_to(ROOT)) for p in ROOT.glob("values/**/*") if p.is_file()}
    assert all(set(v) <= set(elements) for v in ex.values()) and any(ex.values())


@pytest.mark.parametrize(
    ("name", "key"),
    [
        ("odf", "odf"),
        ("cert-manager", "certManager"),
        ("user-monitoring", "userMonitoring"),
        ("a-b-c", "aBC"),
    ],
)
def test_camel(name, key):
    assert cat.camel(name) == key
