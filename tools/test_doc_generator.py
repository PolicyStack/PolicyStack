"""Tests for doc-generator.py. Run: python -m unittest tools/test_doc_generator.py"""

import contextlib
import importlib.util
import io
import re
import tempfile
import textwrap
import unittest
from pathlib import Path
from unittest import mock

_spec = importlib.util.spec_from_file_location("doc_generator", Path(__file__).with_name("doc-generator.py"))
doc_generator = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(doc_generator)


class AnnotationParsingTest(unittest.TestCase):
    def describe(self, values: str, *path):
        with tempfile.NamedTemporaryFile("w", suffix=".yaml", delete=False) as f:
            f.write(textwrap.dedent(values))
        self.addCleanup(Path(f.name).unlink)
        _, comments = doc_generator.YAMLLoader(Path(f.name)).load()
        return doc_generator.DocumentationGenerator().get_field_description(comments, *path)

    def test_single_line(self):
        got = self.describe("""
            stack:
              el:
                # @desc: One line
                enabled: false
            """, "stack", "el", "enabled")
        self.assertEqual(got, "One line")

    def test_consecutive_lines_are_joined(self):
        got = self.describe("""
            stack:
              el:
                config:
                  # @desc: Allow OSDs to be unevenly distributed. Set true on bare metal when the
                  # @desc: count is not a multiple of three.
                  flexibleScaling: null
            """, "stack", "el", "config", "flexibleScaling")
        self.assertEqual(got, "Allow OSDs to be unevenly distributed. Set true on bare metal when the "
                              "count is not a multiple of three.")

    def test_desc_and_description_prefixes_join(self):
        got = self.describe("""
            stack:
              el:
                # @description: First part,
                # @desc: second part
                toggles: {}
            """, "stack", "el", "toggles")
        self.assertEqual(got, "First part, second part")

    def test_plain_comment_starts_a_new_run(self):
        got = self.describe("""
            stack:
              el:
                config:
                  # @desc: Stale annotation
                  # Maintainer note, not rendered.
                  # @desc: Current annotation
                  key: value
            """, "stack", "el", "config", "key")
        self.assertEqual(got, "Current annotation")

    def test_blank_line_starts_a_new_run(self):
        got = self.describe("""
            stack:
              el:
                # @desc: Stale

                # @desc: Current
                enabled: true
            """, "stack", "el", "enabled")
        self.assertEqual(got, "Current")

    def test_multi_line_on_list_item_by_name(self):
        got = self.describe("""
            stack:
              el:
                policies:
                  # @description: Waits for storage nodes.
                  # @description: Requires the storage-nodes element.
                  - name: cluster
                    enabled: true
            """, "stack", "el", "policies", "cluster")
        self.assertEqual(got, "Waits for storage nodes. Requires the storage-nodes element.")

    def test_multi_line_on_key_inside_list_item(self):
        got = self.describe("""
            stack:
              el:
                policies:
                  - name: ready
                    # @desc: Other elements depend on this policy.
                    # @desc: Nothing depends on health.
                    remediationAction: inform
            """, "stack", "el", "policies", 0, "remediationAction")
        self.assertEqual(got, "Other elements depend on this policy. Nothing depends on health.")

    def test_annotation_does_not_leak_to_next_key(self):
        values = """
            stack:
              el:
                # @desc: Documented
                documented: 1
                undocumented: 2
            """
        self.assertEqual(self.describe(values, "stack", "el", "documented"), "Documented")
        self.assertIsNone(self.describe(values, "stack", "el", "undocumented"))


class WriteIfChangedTest(unittest.TestCase):
    OLD = "# el\n\n*Generated: 2026-01-01 00:00:00*\n\nbody\n"

    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.path = Path(tmp.name) / "el.md"
        self.gen = doc_generator.DocumentationGenerator()

    def test_writes_missing_file(self):
        self.assertTrue(self.gen.write_if_changed(self.path, self.OLD))
        self.assertEqual(self.path.read_text(), self.OLD)

    def test_skips_timestamp_only_change(self):
        self.path.write_text(self.OLD)
        new = self.OLD.replace("2026-01-01 00:00:00", "2026-09-25 12:34:56")
        self.assertFalse(self.gen.write_if_changed(self.path, new))
        self.assertEqual(self.path.read_text(), self.OLD)

    def test_writes_content_change(self):
        self.path.write_text(self.OLD)
        new = self.OLD.replace("body", "changed body")
        self.assertTrue(self.gen.write_if_changed(self.path, new))
        self.assertEqual(self.path.read_text(), new)


class ElementReadmeTest(unittest.TestCase):
    STUB = "# Sample App\nPlease see the library chart documentation.\n"

    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.enterContext(contextlib.chdir(tmp.name))
        self.enterContext(contextlib.redirect_stdout(io.StringIO()))
        self.element = Path("stack/my-el")
        self.element.mkdir(parents=True)
        (self.element / "Chart.yaml").write_text("name: my-el\ndescription: Test element\n")
        (self.element / "values.yaml").write_text("stack:\n  myEl:\n    enabled: false\n")
        self.readme = self.element / "README.md"
        self.gen = doc_generator.DocumentationGenerator()

    def test_generate_writes_element_readme(self):
        self.gen.generate_all_docs()
        content = self.readme.read_text()
        self.assertTrue(content.startswith("# my-el - Policy Library Documentation"))
        self.assertIn("(https://github.com/PolicyStack/PolicyStack-chart/tree/main/charts/policy-library)", content)
        self.assertFalse(Path("docs").exists())

    def test_check_passes_after_generate(self):
        self.gen.generate_all_docs()
        self.assertEqual(self.gen.check_all_docs(), 0)

    def test_check_ignores_timestamp(self):
        self.gen.generate_all_docs()
        stale = re.sub(r"\*Generated: [^*]+\*", "*Generated: 2000-01-01 00:00:00*", self.readme.read_text())
        self.readme.write_text(stale)
        self.assertEqual(self.gen.check_all_docs(), 0)

    def test_check_fails_on_stub(self):
        self.readme.write_text(self.STUB)
        self.assertEqual(self.gen.check_all_docs(), 1)

    def test_check_fails_when_missing(self):
        self.assertEqual(self.gen.check_all_docs(), 1)

    def test_element_without_stack_key_is_left_alone(self):
        (self.element / "values.yaml").write_text("stack: {}\n")
        self.readme.write_text(self.STUB)
        self.gen.generate_all_docs()
        self.assertEqual(self.readme.read_text(), self.STUB)
        self.assertEqual(self.gen.check_all_docs(), 0)

    def test_main_single_element(self):
        with mock.patch("sys.argv", ["doc-generator.py", "--stack-dir", "stack", "--element", "my-el"]):
            self.assertEqual(doc_generator.main(), 0)
        self.assertTrue(self.readme.read_text().startswith("# my-el - Policy Library Documentation"))


class OperatorPoliciesTest(unittest.TestCase):
    VALUES = """
        stack:
          myEl:
            enabled: true
            policies:
              - name: install
                enabled: true
              - name: other
                enabled: true
            operatorPolicies:
              # @desc: Second operator
              zeta:
                enabled: true
                policyRef: install
                subscription:
                  name: zeta-operator
                  # @desc: Pinned starting point
                  startingCSV: zeta.v1.0.0
                upgradeApproval: Automatic
                # @desc: Approved CSVs
                versions:
                  - zeta.v1.0.0
                  - zeta.v1.1.0
              alpha:
                enabled: true
                description: First operator
                policyRef: install
                upgradeApproval: None
                versions: []
              orphan:
                enabled: true
                policyRef: missing
              disabled:
                enabled: false
                policyRef: other
              unset: null
        """

    def render(self, values: str) -> str:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        element = Path(tmp.name) / "my-el"
        element.mkdir()
        (element / "values.yaml").write_text(textwrap.dedent(values))
        return doc_generator.DocumentationGenerator().generate_element_docs(element)

    def test_key_is_name_and_yaml_order_kept(self):
        md = self.render(self.VALUES)
        self.assertIn("| Name | `install-zeta` |", md)
        self.assertIn("| Name | `install-alpha` |", md)
        self.assertLess(md.index("Operator: zeta"), md.index("Operator: alpha"))

    def test_desc_on_key_and_nested_keys(self):
        md = self.render(self.VALUES)
        self.assertIn("###### 🔧 Operator: zeta\n> Second operator", md)
        self.assertIn("###### 🔧 Operator: alpha\n> First operator", md)
        self.assertIn("| Starting CSV | `zeta.v1.0.0` | Pinned starting point |", md)

    def test_versions(self):
        md = self.render(self.VALUES)
        self.assertIn("**Approved Versions:**\n*Approved CSVs*\n- `zeta.v1.0.0`\n- `zeta.v1.1.0`\n", md)
        self.assertEqual(md.count("**Approved Versions:**"), 1)

    def test_grouping_orphans_and_stats(self):
        md = self.render(self.VALUES)
        other = md[md.index("Policy: other"):]
        self.assertNotIn("Operator:", other[:other.index("---")])
        self.assertIn("### Orphaned Operator Policies", md)
        self.assertIn("- orphan\n", md)
        self.assertIn("| Operator Policies | 3 |", md)

    def test_list_shape_rejected(self):
        with self.assertRaisesRegex(ValueError, "operatorPolicies must be a map"):
            self.render("""
                stack:
                  myEl:
                    enabled: true
                    operatorPolicies:
                      - name: old
                """)


if __name__ == "__main__":
    unittest.main()
