"""Tests for doc-generator.py. Run: python -m unittest tools/test_doc_generator.py"""

import contextlib
import importlib.util
import io
import tempfile
import textwrap
import unittest
from pathlib import Path

_spec = importlib.util.spec_from_file_location(
    "doc_generator", Path(__file__).with_name("doc-generator.py")
)
doc_generator = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(doc_generator)

BASE = """
stack:
  el:
    enabled: false
    default:
      remediationAction: enforce
    policies:
      install:
        description: "Install the operator"
      config:
        description: "Configure it"
        remediationAction: inform
        severity: high
        dependencies:
          - name: install
          - name: ready
            element: storage-nodes
          - name: other-x
            release: other-x
            compliance: NonCompliant
        templates:
          el-config:
            description: "Example ConfigMap"
            remediationAction: enforce
            waitForOperator: el
          el-ready:
            remediationAction: InformOnly
            severity: critical
            extraDependencies:
              - name: el-config
              - name: raw-name
                raw: true
    operatorPolicies:
      el:
        description: "Example operator"
        namespace: el-ns
        subscription: {name: el-operator, channel: stable}
        versions: []
"""

CONVERTERS = {
    "config/el-config.yaml": "kind: ConfigMap\n",
    "config/el-ready.yaml": "kind: ConfigMap\n",
    "config/_partial.yaml": "kind: Partial\n",
    "_shared/x.yaml": "kind: Shared\n",
}


def element_dir(root, values, chart=None, name="el", converters=None):
    path = Path(root) / name
    path.mkdir()
    (path / "values.yaml").write_text(textwrap.dedent(values))
    (path / "Chart.yaml").write_text(
        textwrap.dedent(chart or f"name: {name}\ndescription: An element\n")
    )
    for file, text in (CONVERTERS if converters is None else converters).items():
        (path / "converters" / file).parent.mkdir(parents=True, exist_ok=True)
        (path / "converters" / file).write_text(text)
    return path


class Fixture(unittest.TestCase):
    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.root = Path(tmp.name)

    def render(self, values=BASE, chart=None, converters=None):
        return doc_generator.generate(
            element_dir(self.root, values, chart, converters=converters)
        )

    def fresh(self, values):
        """Render a second element in its own directory."""
        with tempfile.TemporaryDirectory() as tmp:
            return doc_generator.generate(element_dir(tmp, values))

    def run_main(self, *args):
        with contextlib.redirect_stdout(io.StringIO()):
            return doc_generator.main(["--stack-dir", str(self.root), *args])


class CommentsTest(unittest.TestCase):
    def comments(self, text):
        return doc_generator.comments(textwrap.dedent(text))

    def test_single_and_multi_line(self):
        got = self.comments("""
            config:
              # One line
              a: 1
              # First part,
              # second part
              b: 2
            """)
        self.assertEqual(
            got,
            {("config", "a"): "One line", ("config", "b"): "First part, second part"},
        )

    def test_blank_line_ends_block(self):
        got = self.comments("""
            config:
              # Maintainer note

              # Description
              a: 1
            """)
        self.assertEqual(got[("config", "a")], "Description")

    def test_first_key_stops_at_parent(self):
        got = self.comments("""
            # Section intro
            config:
              a: 1
            """)
        self.assertEqual(got, {("config",): "Section intro"})

    def test_nested_and_no_leak(self):
        got = self.comments("""
            config:
              # Parent
              auth:
                # Child
                provider: x
                other: y
              plain: 1
            """)
        self.assertEqual(
            got, {("config", "auth"): "Parent", ("config", "auth", "provider"): "Child"}
        )

    def test_flow_mapping_children_not_attributed(self):
        got = self.comments("""
            config:
              # Sizing
              scanner: {min: 2, max: 5}
            """)
        self.assertEqual(got, {("config", "scanner"): "Sizing"})

    def test_only_comments_at_key_indent(self):
        got = self.comments("""
            config:
              script: |
                echo hi
                # not a comment
              b: 1
              a:
                x: 1
                # y: 2
              c: 1
            """)
        self.assertEqual(got, {})

    def test_list_items_ignored(self):
        got = self.comments("""
            items:
              # Not a key
              - name: a
            """)
        self.assertEqual(got, {})


class HelpersTest(unittest.TestCase):
    def test_cell_escapes(self):
        self.assertEqual(
            doc_generator.cell("api.<domain> | `a<b|c`"), r"api.&lt;domain> \| `a<b\|c`"
        )
        self.assertEqual(doc_generator.cell("a<br>b"), "a<br>b")
        self.assertEqual(doc_generator.cell("two\n lines"), "two lines")

    def test_flow(self):
        self.assertEqual(doc_generator.flow(""), "''")
        self.assertEqual(doc_generator.flow(None), "null")
        self.assertEqual(doc_generator.flow(False), "false")
        self.assertEqual(doc_generator.flow({"z": 1, "a": 2}), "{z: 1, a: 2}")

    def test_table_drops_empty_columns(self):
        self.assertEqual(
            doc_generator.table(["A", "B"], [("x", "")]), ["| A |", "|---|", "| x |"]
        )

    def test_camel(self):
        self.assertEqual(doc_generator.camel("cert-manager"), "certManager")


class RenderTest(Fixture):
    def test_header_and_quick_start(self):
        md = self.render(
            chart="name: el\ndescription: An <x> element\nhome: https://example.com/el/\n"
        )
        self.assertTrue(
            md.startswith(doc_generator.HEADER + "\n# el\n\nAn &lt;x> element\n")
        )
        self.assertIn("Guide: https://example.com/el/", md)
        self.assertIn("stack:\n  el:\n    enabled: true", md)
        self.assertNotIn("Generated:", md)

    def test_no_guide_without_home(self):
        self.assertNotIn("Guide:", self.render())

    def test_stability(self):
        md = self.render(
            chart="name: el\nannotations:\n  policystk.io/stability: preview\n"
        )
        self.assertIn(
            "\n\nStability: preview ([Compatibility](https://policystk.io/"
            "reference/compatibility/#element-stability))\n\n",
            md,
        )

    def test_no_stability_without_annotation(self):
        self.assertNotIn("Stability:", self.render(chart="name: el\nannotations:\n"))

    def test_unknown_stability(self):
        with self.assertRaisesRegex(ValueError, "policystk.io/stability is 'beta'"):
            self.render(
                chart="name: el\nannotations:\n  policystk.io/stability: beta\n"
            )

    def test_names_and_dependencies(self):
        md = self.render()
        self.assertIn(
            "`config-el-<cluster>`: Configure it. Depends on `install-el-<cluster>`, "
            "`ready-storage-nodes-<cluster>`, `other-x-other-x` (NonCompliant).",
            md,
        )
        self.assertIn("`install-el-status`<br>compliant while pending", md)
        self.assertIn("`config-el-config`<br>`raw-name` |", md)

    def test_ignore_pending_override(self):
        md = self.render(
            BASE.replace(
                "waitForOperator: el",
                "waitForOperator: el\n            ignorePending: false",
            )
        )
        self.assertIn("| `install-el-status` |", md)
        self.assertNotIn("compliant while pending", md)

    def test_discovery(self):
        md = self.render()
        self.assertIn("| `config-el-config` | inform | high | `install-el-status`", md)
        self.assertIn("[el-ready](converters/config/el-ready.yaml)", md)
        # _ files and directories are partials, never templates or policies.
        self.assertNotIn("partial", md)
        self.assertNotIn("_shared", md)

    def test_discovered_only_policy(self):
        md = self.render(converters={**CONVERTERS, "extra/thing.yaml": "kind: X\n"})
        self.assertIn("### Policy `extra`\n\n`extra-el-<cluster>`\n", md)
        self.assertIn("| `extra-thing` | enforce | low |", md)

    def test_file_reuse(self):
        md = self.render(
            BASE.replace(
                "          el-ready:\n",
                "          el-ready:\n            file: [config/el-config, _shared/x]\n",
            ),
            converters={
                "config/el-config.yaml": "kind: A\n",
                "_shared/x.yaml": "kind: B\n",
            },
        )
        self.assertIn(
            "[el-config](converters/config/el-config.yaml)<br>[x](converters/_shared/x.yaml)",
            md,
        )

    def test_template_without_file_rejected(self):
        with self.assertRaisesRegex(
            ValueError, "policies.config.templates.el-ready has no file"
        ):
            self.render(converters={"config/el-config.yaml": "kind: A\n"})

    def test_policy_without_templates_rejected(self):
        with self.assertRaisesRegex(ValueError, "policies.ghost has no templates"):
            self.render(
                BASE.replace("    policies:\n", "    policies:\n      ghost: {}\n")
            )

    def test_order_manifests(self):
        md = self.render(
            BASE.replace(
                "        severity: high\n",
                "        severity: high\n        orderManifests: [el-ready]\n",
            )
        )
        self.assertIn("Applies `el-ready` first, in order, then the rest.", md)
        self.assertLess(md.index("`config-el-ready`"), md.index("`config-el-config`"))
        with self.assertRaisesRegex(ValueError, "orderManifests names"):
            self.fresh(
                BASE.replace(
                    "severity: high\n",
                    "severity: high\n        orderManifests: [nope]\n",
                )
            )

    def test_defaults_chain(self):
        md = self.render()
        # Template InformOnly; otherwise the Policy action, then default.remediationAction.
        self.assertIn("| `config-el-config` | inform | high |", md)
        self.assertIn("| `config-el-ready` | inform (always) | critical |", md)
        self.assertIn("| `install-el` | enforce | low |", md)
        md = self.fresh(
            BASE.replace(
                "    default:\n      remediationAction: enforce\n",
                "    default:\n      severity: medium\n",
            )
        )
        self.assertIn("| `install-el` | inform | medium |", md)

    def test_operators_in_install(self):
        # No converters and no policies key: the operator alone makes Policy install.
        values = (
            BASE[: BASE.index("    policies:")]
            + BASE[BASE.index("    operatorPolicies:") :]
        )
        md = self.render(values, converters={})
        self.assertIn("### Policy `install`\n\n`install-el-<cluster>`\n", md)
        self.assertIn(
            "| `install-el` | enforce | low | [operator](#operators) `el-operator` | Example operator |",
            md,
        )
        self.assertNotIn("### Policy `config`", md)

    def test_renders_compliance_type(self):
        md = self.render(
            BASE.replace(
                "          el-ready:\n",
                "          el-ready:\n            complianceType: mustonlyhave\n",
            )
        )
        self.assertIn("[el-ready](converters/config/el-ready.yaml) (mustonlyhave)", md)
        self.assertIn("[el-config](converters/config/el-config.yaml) |", md)

    def test_raw_converter_compliance_types(self):
        md = self.render(
            BASE.replace(
                "          el-ready:\n",
                "          el-ready:\n            complianceType: mustonlyhave\n",
            ),
            converters={
                **CONVERTERS,
                "config/el-ready.yaml": "{{- range .Config.x }}\n- complianceType: musthave\n"
                "- complianceType: mustnothave\n- complianceType: 'mustonlyhave'\n{{- end }}\n",
                "config/el-config.yaml": "kind: A\n---\nkind: B\n",
            },
        )
        self.assertIn(
            "[el-ready](converters/config/el-ready.yaml) (mustnothave, mustonlyhave)",
            md,
        )
        self.assertIn("[el-config](converters/config/el-config.yaml) |", md)

    def test_operators_table(self):
        md = self.render()
        self.assertIn(
            "| `install-el` | `el-operator` | `stable` | `redhat-operators` | `el-ns` | any |",
            md,
        )
        md = self.fresh(
            BASE.replace(
                "versions: []",
                "upgradeApproval: None\n        versions: [el.v1, el.v2]",
            )
        )
        self.assertIn("| `el.v1`<br>`el.v2`, None approval |", md)

    def test_no_optional_or_settings_sections_when_absent(self):
        md = self.render()
        self.assertNotIn("## Optional pieces", md)
        self.assertNotIn("## Settings", md)

    def test_compliance_line(self):
        md = self.render(
            BASE.replace(
                "    default:\n",
                "    default:\n      standards: [NIST SP 800-53]\n      controls: [CM-2 Baseline]\n",
                1,
            )
        )
        self.assertIn("Standards: NIST SP 800-53. Controls: CM-2 Baseline.", md)

    def test_pre_4_shape_rejected(self):
        for values in (
            "stack:\n  el:\n    operatorPolicies:\n      - name: x\n",
            "stack:\n  el:\n    policies:\n      - name: x\n",
            "stack:\n  el:\n    configPolicies: []\n",
        ):
            with self.assertRaisesRegex(TypeError, "policy-library 4.0"):
                self.fresh(values)

    def test_element_without_stack_key(self):
        self.assertIsNone(self.render("stack:\n  other: {enabled: true}\n"))


OPTIONAL = (
    BASE.replace(
        """      config:
        description: "Configure it"
""",
        """      config:
        description: "Configure it"
        enabled: false
""",
    )
    .replace(
        "            severity: critical\n",
        "            severity: critical\n            enabled: false\n",
    )
    .replace(
        '        description: "Example operator"\n',
        '        description: "Example operator"\n        enabled: false\n',
    )
)


class OptionalTest(Fixture):
    def test_section(self):
        md = self.render(OPTIONAL)
        self.assertIn("## Optional pieces\n\nShipped with `enabled: false`.", md)
        for row in [
            "| Policy [`config`](#policy-config) | Configure it | `policies.config.enabled: true` |",
            (
                "| template `config-el-ready` (needs Policy `config`) |  | "
                "`policies.config.templates.el-ready.enabled: true` |"
            ),
            "| operator `install-el` | Example operator | `operatorPolicies.el.enabled: true` |",
        ]:
            self.assertIn(row, md)

    def test_comment_describes_entry(self):
        md = self.render(
            OPTIONAL.replace(
                "          el-ready:\n",
                "          # The ready check\n          el-ready:\n",
            )
        )
        self.assertIn("(needs Policy `config`) | The ready check |", md)

    def test_state(self):
        md = self.render(OPTIONAL)
        self.assertIn(
            "Off by default; turn on with `policies.config.enabled: true`.", md
        )
        self.assertIn("| `config-el-ready` (off) |", md)
        self.assertIn("| `install-el` (off) |", md)
        self.assertIn("Renders nothing by default: every template is off.", md)

    def test_null_enabled_is_on(self):
        md = self.render(OPTIONAL.replace("enabled: false", "enabled: null"))
        self.assertNotIn("## Optional pieces", md)
        self.assertNotIn("Off by default", md)


SETTINGS = BASE.replace(
    "    enabled: false\n",
    """    enabled: false
    # Site data
    config:
      # Log level
      level: info
      # Sizing
      scanner:
        min: 2
        max: 5
      # Auth
      auth:
        # Provider
        provider: openshift
        groups: [a, b]
      # Pools keyed by name
      pools:
        first: {addresses: [10.0.0.1-10.0.0.9], autoAssign: true, avoidBuggyIPs: true}
        second: {addresses: [10.0.1.1-10.0.1.9]}
      empty: {}
      unset: null
      script: |
        line1
        line2
""",
    1,
)


class SettingsTest(Fixture):
    def setUp(self):
        super().setUp()
        self.md = self.render(SETTINGS)

    def test_intro_and_fixed_line(self):
        self.assertIn(
            "## Settings\n\nSite data\n\nUnder `stack.el.config`. Maps merge across layers; "
            "lists are replaced whole.",
            self.md,
        )

    def test_rows(self):
        for row in [
            "| `level` | Log level | `info` |",
            "| `scanner` | Sizing |  |",  # all-scalar map expands
            "| `scanner.min` |  | `2` |",
            "| `auth` | Auth |  |",  # a commented child expands
            "| `auth.provider` | Provider | `openshift` |",
            "| `auth.groups` |  | `[a, b]` |",
            "| `pools` | Pools keyed by name | 2 entries, below |",
            "| `empty` |  | `{}` |",
            "| `unset` |  | `null` |",
        ]:
            self.assertIn(row, self.md)

    def test_multi_line_string_in_details(self):
        self.assertIn("| `script` |  | below |", self.md)
        self.assertIn(
            "<code>script</code> default</summary>\n\n```\nline1\nline2\n```", self.md
        )

    def test_long_default_in_details(self):
        self.assertIn(
            "<details><summary><code>pools</code> default</summary>\n\n```yaml\nfirst:\n",
            self.md,
        )
        self.assertIn("```\n\n</details>", self.md)


class MainTest(Fixture):
    def setUp(self):
        super().setUp()
        self.el = element_dir(self.root, BASE)

    def test_generate_then_check(self):
        self.assertEqual(self.run_main(), 0)
        self.assertTrue(
            (self.el / "README.md").read_text().startswith(doc_generator.HEADER)
        )
        self.assertEqual(self.run_main("--check"), 0)

    def test_check_fails_when_missing_or_stale(self):
        self.assertEqual(self.run_main("--check"), 1)
        (self.el / "README.md").write_text("stub\n")
        self.assertEqual(self.run_main("--check"), 1)
        self.assertEqual((self.el / "README.md").read_text(), "stub\n")

    def test_rerun_writes_nothing(self):
        self.run_main()
        self.assertFalse(
            doc_generator.write_if_changed(
                self.el / "README.md", doc_generator.generate(self.el)
            )
        )

    def test_single_element(self):
        self.assertEqual(self.run_main("--element", "el"), 0)
        self.assertTrue((self.el / "README.md").exists())
        self.assertEqual(self.run_main("--element", "missing"), 1)

    def test_element_without_values_skipped(self):
        (self.root / "empty").mkdir()
        self.assertEqual(self.run_main(), 0)
        self.assertFalse((self.root / "empty" / "README.md").exists())

    def test_missing_stack_dir(self):
        def main(*args):
            with contextlib.redirect_stdout(io.StringIO()):
                return doc_generator.main(list(args))

        # A framework copy before its first element: nothing to do.
        with contextlib.chdir(self.root / "el"):
            self.assertEqual(main(), 0)
            self.assertEqual(main("--check"), 0)
            self.assertEqual(main("--element", "el"), 1)
        self.assertEqual(self.run_main("--stack-dir", str(self.root / "nope")), 1)


if __name__ == "__main__":
    unittest.main()
