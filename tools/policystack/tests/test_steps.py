"""Upgrade step glue: the path guard refuses (never skips), and nothing is written by apply."""

import pytest

from policystack import migrate

STEP = {
    "summary": "s",
    "files": ["values/**/*.yaml", "fleet/*.yaml"],
    "ops": [
        {
            "rename": "stack.userMonitoring.config.prometheus.retention",
            "to": "retentionTime",
        }
    ],
}
SRC = "stack:\n  userMonitoring:\n    config:\n      prometheus:\n        # keep\n        retention: 24h\n"


def tracked(root):
    """Every file under root, as git ls-files would list them all."""
    return {p.relative_to(root).as_posix() for p in root.rglob("*")}


def targets(root, globs):
    return migrate.targets(root, globs, tracked(root))


def apply(root, steps):
    return migrate.apply(root, steps, tracked(root))


@pytest.fixture
def root(tmp_path):
    (tmp_path / "values" / "clusters").mkdir(parents=True)
    (tmp_path / "fleet").mkdir()
    (tmp_path / "values" / "clusters" / "a.yaml").write_text(SRC)
    (tmp_path / "values" / "b.yaml").write_text("stack:\n  odf:\n    enabled: true\n")
    (tmp_path / "fleet" / "a.yaml").write_text("revision: main\n")
    return tmp_path


def test_targets(root):
    assert [p.relative_to(root).as_posix() for p in targets(root, STEP["files"])] == [
        "fleet/a.yaml",
        "values/b.yaml",
        "values/clusters/a.yaml",
    ]


@pytest.mark.parametrize(
    "glob",
    [
        "../x/*.yaml",
        "values/../stack/*.yaml",
        "/etc/*.conf",
        "stack/*/values.yaml",
        "*.yaml",
        "values.yaml",
    ],
)
def test_guard_refuses_patterns_and_paths(root, glob):
    (root / "stack" / "e").mkdir(parents=True)
    (root / "stack" / "e" / "values.yaml").write_text("x: 1\n")
    (root / "values.yaml").write_text("x: 1\n")
    with pytest.raises(ValueError):
        targets(root, [glob])


def test_guard_refuses_symlinks(root, tmp_path_factory):
    outside = tmp_path_factory.mktemp("outside")
    (outside / "o.yaml").write_text("x: 1\n")
    (root / "values" / "link.yaml").symlink_to(outside / "o.yaml")
    with pytest.raises(ValueError, match="symlink"):
        targets(root, ["values/*.yaml"])
    (root / "values" / "link.yaml").unlink()
    (root / "values" / "dir").symlink_to(outside)
    with pytest.raises(ValueError, match="symlink"):
        targets(root, ["values/dir/*.yaml"])


def test_apply_changes_only_matching_files(root):
    before = {p: p.read_bytes() for p in root.rglob("*.yaml")}
    out = apply(root, [("s.yaml", STEP), ("manual.yaml", {"summary": "m", "manual": "do it"})])
    assert list(out) == [root / "values" / "clusters" / "a.yaml"]
    assert out[root / "values" / "clusters" / "a.yaml"] == SRC.replace(
        "retention:", "retentionTime:"
    )
    assert {p: p.read_bytes() for p in root.rglob("*.yaml")} == before  # apply writes nothing


def test_apply_absent_key_is_byte_identical(root):
    assert (
        apply(
            root,
            [
                (
                    "s.yaml",
                    {**STEP, "ops": [{"rename": "stack.nothing.here", "to": "x"}]},
                )
            ],
        )
        == {}
    )


def test_apply_chains_steps_on_one_file(root):
    second = {
        **STEP,
        "ops": [
            {
                "rename": "stack.userMonitoring.config.prometheus.retentionTime",
                "to": "keep",
            }
        ],
    }
    out = apply(root, [("1.yaml", STEP), ("2.yaml", second)])
    assert out[root / "values" / "clusters" / "a.yaml"] == SRC.replace("retention:", "keep:")


def test_apply_refusal_names_step_file_and_manual(root):
    (root / "values" / "bad.yaml").write_text(
        "stack:\n  userMonitoring:\n    config:\n      prometheus:\n        retention: 1\n...\n"
    )
    with pytest.raises(ValueError, match=r"(?s)s\.yaml: values/bad\.yaml: .*edit it by hand"):
        apply(root, [("s.yaml", {**STEP, "manual": "edit it by hand"})])


def test_apply_skips_untracked(root):
    """safety/D5: an ignored (untracked) values file is never rewritten, so git add can't fail on it."""
    assert migrate.apply(root, [("s.yaml", STEP)], {"values/b.yaml"}) == {}


@pytest.mark.parametrize("text", ["stack:\n  enabled: true\n  enabled: false\n", "a: [1, 2\n"])
def test_apply_unparsable_file_is_a_refusal_naming_it(root, text):
    """safety/D11: duplicate keys and syntax errors refuse the step with its name and the file."""
    (root / "values" / "bad.yaml").write_text(text)
    with pytest.raises(ValueError, match=r"^s\.yaml: values/bad\.yaml: "):
        apply(root, [("s.yaml", STEP)])


def test_stack_keys():
    """correctness/D1: the elements a step's ops rewrite, so the update moves them with it."""
    assert migrate.stack_keys(STEP["ops"]) == {"userMonitoring"}
    assert migrate.stack_keys(
        [
            {"move": "stack.*.toggles.*", "to": "x"},
            {"rename": ["stack", "odf", "a.b"], "to": "c"},
            {"rename": "hubName", "to": "hub"},
        ]
    ) == {"*", "odf"}
