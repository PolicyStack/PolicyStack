import textwrap

import pytest
from ruamel.yaml import YAMLError

from policystack.migrate import migrate

R = [{"rename": "stack.oldEl", "to": "newEl"}]


def d(s):
    return textwrap.dedent(s).lstrip("\n")


CASES = {
    # comments and blank lines: only the key token changes
    "comments": (
        R,
        d("""
        # header
        stack:
          # above oldEl

          # right above oldEl
          oldEl:   # inline on key
            enabled: true  # inline on value

          # trailing comment, describes next
          other:
            enabled: false
        # footer
        """),
        d("""
        # header
        stack:
          # above oldEl

          # right above oldEl
          newEl:   # inline on key
            enabled: true  # inline on value

          # trailing comment, describes next
          other:
            enabled: false
        # footer
        """),
    ),
    "first-and-last-key": (
        R,
        "stack:\n  oldEl: {enabled: true}\n",
        "stack:\n  newEl: {enabled: true}\n",
    ),
    "flow-style": (
        R,
        "stack: {oldEl: {enabled: true}, other: {enabled: false}}\n",
        "stack: {newEl: {enabled: true}, other: {enabled: false}}\n",
    ),
    "null-explicit": (
        R,
        "stack:\n  oldEl: null\n  keep: null\n  bare:\n  tilde: ~\n",
        "stack:\n  newEl: null\n  keep: null\n  bare:\n  tilde: ~\n",
    ),
    "null-tilde-renamed": (R, "stack:\n  oldEl: ~\n", "stack:\n  newEl: ~\n"),
    "null-bare-renamed": (
        R,
        "stack:\n  oldEl:\n  x: 1\n",
        "stack:\n  newEl:\n  x: 1\n",
    ),
    "quotes": (
        R,
        d("""
        stack:
          oldEl:
            a: 'single'
            b: "double"
            c: '{{ .Values.x }}'
            d: "{{hub .ManagedClusterName hub}}"
            e: |
              {{- range .x }}
              {{ . }}
              {{- end }}
        """),
        None,
    ),
    "anchors-aliases-merge": (
        R,
        d("""
        defaults: &on
          enabled: true
        stack:
          oldEl: *on
          other:
            <<: *on
            extra: 1
        """),
        None,
    ),
    "multi-doc": (
        R,
        d("""
        ---
        stack:
          oldEl: {enabled: true}
        ---
        # second
        stack:
          oldEl:
            enabled: false
        """),
        None,
    ),
    "multi-doc-no-leading-marker": (
        R,
        "stack:\n  oldEl: 1\n---\nstack:\n  x: 1\n",
        "stack:\n  newEl: 1\n---\nstack:\n  x: 1\n",
    ),
    "seq-indent-0": (
        R,
        d("""
        list:
        - a
        - b
        stack:
          oldEl:
            items:
            - x
        """),
        None,
    ),
    "no-trailing-newline": (R, "# c\nstack:\n  oldEl: 1", "# c\nstack:\n  newEl: 1"),
    "long-line": (R, "stack:\n  oldEl:\n    note: " + "word " * 40 + "end\n", None),
    "crlf": (
        R,
        "stack:\r\n  oldEl: 1\r\n  x: 2\r\n",
        "stack:\r\n  newEl: 1\r\n  x: 2\r\n",
    ),
    "wildcard-toggles-to-policies": (
        [
            {"move": "stack.*.toggles.*", "to": "stack.$1.toggles.$2.enabled"},
            {"rename": "stack.*.toggles", "to": "policies"},
        ],
        d("""
        stack:
          gitops:
            enabled: true
            # toggles comment
            toggles:
              instance: false   # off
              link: false
          acm:
            enabled: true
        """),
        d("""
        stack:
          gitops:
            enabled: true
            # toggles comment
            policies:
              instance:
                enabled: false  # off
              link:
                enabled: false
          acm:
            enabled: true
        """),
    ),
    "move-to-other-parent": (
        [{"move": "stack.x.config.old", "to": "stack.x.config.groups.new"}],
        "stack:\n  x:\n    config:\n      a: 1\n      old: v  # why\n      b: 2\n",
        "stack:\n  x:\n    config:\n      a: 1\n      groups:\n        new: v # why\n      b: 2\n",
    ),
}

ABSENT = {
    "absent-plain": "stack:\n  other: {enabled: true}\n",
    "absent-no-stack": "# only a comment\n",
    "absent-empty": "",
    "absent-commented-out": "#stack:\n#  oldEl:\n#    enabled: true\n",
    "absent-null-parent": "stack:\n",
    "absent-weird-format": "stack:\n    other:   { enabled:   yes }\nlist:\n- 1\n",
}


@pytest.mark.parametrize("name", CASES)
def test_case(name):
    ops, src, want = CASES[name]
    got = migrate(src, ops)
    if want is None:  # expect only the key token to change
        want = src.replace("oldEl", "newEl")
    assert got == want


@pytest.mark.parametrize("name", ABSENT)
@pytest.mark.parametrize(
    "ops",
    [
        R,
        [{"move": "stack.acs.config.auth.adminGroup", "to": "stack.acs.x"}],
        [{"move": "stack.*.toggles.*", "to": "stack.$1.toggles.$2.enabled"}],
    ],
)
def test_absent_is_untouched(name, ops):
    src = ABSENT[name]
    assert migrate(src, ops) is src


def test_collision():
    with pytest.raises(ValueError, match="already set"):
        migrate("stack:\n  oldEl: 1\n  newEl: 2\n", R)


def test_idempotent():
    once = migrate("stack:\n  oldEl: 1\n", R)
    assert migrate(once, R) is once


def test_template_unquoted_refused():
    with pytest.raises((ValueError, YAMLError)):
        migrate("stack:\n  oldEl: {{ .Values.x }}\n", R)


def test_merge_inherited_key_untouched():
    src = "defaults: &on\n  enabled: true\nstack:\n  other:\n    <<: *on\n    extra: 1\n"
    assert migrate(src, [{"rename": "stack.other.enabled", "to": "on"}]) is src


def test_list_path_for_dotted_keys():
    src = "stack:\n  e:\n    labels:\n      a.b/c: x\n"
    assert (
        migrate(src, [{"rename": ["stack", "e", "labels", "a.b/c"], "to": "d"}])
        == "stack:\n  e:\n    labels:\n      d: x\n"
    )


@pytest.mark.parametrize(
    "src",
    [
        "stack:\n  oldEl:\n    note: a long\n      plain scalar\n",  # ruamel joins the scalar: line count changes
        "stack: {oldEl: 1,\n  x: 2}\n",  # multi-line flow map
        "stack:\n  oldEl: 1\n...\n",  # document end marker
        "stack:\n  oldEl: 1\t# tab before comment\n",  # ruamel scanner rejects it
    ],
)
def test_refused_not_corrupted(src):
    with pytest.raises((ValueError, YAMLError)):
        migrate(src, R)


def test_library_4_toggles_shape():
    ops = [
        {"move": "stack.*.toggles.*", "to": "stack.$1.toggles.$2.enabled"},
        {
            "move": "stack.gitops.toggles.ca-bundle",
            "to": "stack.gitops.toggles.instance.templates.ca-bundle",
        },
        {"rename": "stack.*.toggles", "to": "policies"},
    ]
    src = "stack:\n  gitops:\n    enabled: true\n    toggles:\n      instance: false\n      ca-bundle: false\n      link: false\n"
    want = (
        "stack:\n  gitops:\n    enabled: true\n    policies:\n      instance:\n        enabled: false\n"
        "        templates:\n          ca-bundle:\n            enabled: false\n      link:\n        enabled: false\n"
    )
    out = migrate(src, ops)
    assert out == want
    assert migrate(out, ops) is out
