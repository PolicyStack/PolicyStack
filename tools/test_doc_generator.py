"""Tests for doc-generator.py. Run: python -m unittest tools/test_doc_generator.py"""

import contextlib
import importlib.util
import io
import tempfile
import textwrap
import unittest
from pathlib import Path

_spec = importlib.util.spec_from_file_location("doc_generator", Path(__file__).with_name("doc-generator.py"))
doc_generator = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(doc_generator)

BASE = """
stack:
  el:
    enabled: false
    policies:
      - name: install
        enabled: true
        description: "Install the operator"
        remediationAction: enforce
      - name: config
        enabled: true
        description: "Configure it"
        dependencies:
          - name: install
          - name: ready
            element: storage-nodes
          - name: other-x
            release: other-x
            compliance: NonCompliant
    operatorPolicies:
      el:
        enabled: true
        description: "Example operator"
        policyRef: install
        namespace: el-ns
        subscription: {name: el-operator, channel: stable, source: redhat-operators}
        upgradeApproval: Automatic
        versions: []
    configPolicies:
      - name: el-config
        enabled: true
        description: "Example ConfigMap"
        policyRef: config
        remediationAction: enforce
        waitForOperator: el
        ignorePending: true
        templateNames:
          - name: el-configmap
          - name: el-role
            complianceType: mustonlyhave
      - name: el-ready
        enabled: true
        policyRef: config
        remediationAction: InformOnly
        extraDependencies:
          - name: el-config
          - name: raw-name
            raw: true
"""


def element_dir(root, values, chart=None, name='el'):
    path = Path(root) / name
    path.mkdir()
    (path / 'values.yaml').write_text(textwrap.dedent(values))
    (path / 'Chart.yaml').write_text(textwrap.dedent(chart or f"name: {name}\ndescription: An element\n"))
    return path


class Fixture(unittest.TestCase):
    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.root = Path(tmp.name)

    def render(self, values=BASE, chart=None):
        return doc_generator.generate(element_dir(self.root, values, chart))

    def fresh(self, values):
        """Render a second element in its own directory."""
        with tempfile.TemporaryDirectory() as tmp:
            return doc_generator.generate(element_dir(tmp, values))

    def run_main(self, *args):
        with contextlib.redirect_stdout(io.StringIO()):
            return doc_generator.main(['--stack-dir', str(self.root), *args])


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
        self.assertEqual(got, {('config', 'a'): 'One line', ('config', 'b'): 'First part, second part'})

    def test_blank_line_ends_block(self):
        got = self.comments("""
            config:
              # Maintainer note

              # Description
              a: 1
            """)
        self.assertEqual(got[('config', 'a')], 'Description')

    def test_first_key_stops_at_parent(self):
        got = self.comments("""
            # Section intro
            config:
              a: 1
            """)
        self.assertEqual(got, {('config',): 'Section intro'})

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
        self.assertEqual(got, {('config', 'auth'): 'Parent', ('config', 'auth', 'provider'): 'Child'})

    def test_flow_mapping_children_not_attributed(self):
        got = self.comments("""
            config:
              # Sizing
              scanner: {min: 2, max: 5}
            """)
        self.assertEqual(got, {('config', 'scanner'): 'Sizing'})

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
        self.assertEqual(doc_generator.cell('api.<domain> | `a<b|c`'), r'api.&lt;domain> \| `a<b\|c`')
        self.assertEqual(doc_generator.cell('a<br>b'), 'a<br>b')
        self.assertEqual(doc_generator.cell('two\n lines'), 'two lines')

    def test_flow(self):
        self.assertEqual(doc_generator.flow(''), "''")
        self.assertEqual(doc_generator.flow(None), 'null')
        self.assertEqual(doc_generator.flow(False), 'false')
        self.assertEqual(doc_generator.flow({'z': 1, 'a': 2}), '{z: 1, a: 2}')

    def test_table_drops_empty_columns(self):
        self.assertEqual(doc_generator.table(['A', 'B'], [('x', '')]), ['| A |', '|---|', '| x |'])

    def test_camel(self):
        self.assertEqual(doc_generator.camel('cert-manager'), 'certManager')


class RenderTest(Fixture):
    def test_header_and_quick_start(self):
        md = self.render(chart="name: el\ndescription: An <x> element\nhome: https://example.com/el/\n")
        self.assertTrue(md.startswith(doc_generator.HEADER + '\n# el\n\nAn &lt;x> element\n'))
        self.assertIn('Guide: https://example.com/el/', md)
        self.assertIn('stack:\n  el:\n    enabled: true', md)
        self.assertNotIn('Generated:', md)

    def test_no_guide_without_home(self):
        self.assertNotIn('Guide:', self.render())

    def test_names_and_dependencies(self):
        md = self.render()
        self.assertIn('`config-el-<cluster>`: Configure it. Depends on `install-el-<cluster>`, '
                      '`ready-storage-nodes-<cluster>`, `other-x-other-x` (NonCompliant).', md)
        self.assertIn('`install-el-status`<br>compliant while pending', md)
        self.assertIn('`config-el-config`<br>`raw-name`', md)

    def test_actions(self):
        md = self.render()
        self.assertIn('| `config-el-config` | enforce |', md)
        self.assertIn('| `config-el-ready` | inform (always) |', md)
        # The operator inherits the Policy's action.
        self.assertIn('| `install-el` | enforce |', md)

    def test_policy_action_overrides_template(self):
        md = self.render(BASE.replace('description: "Configure it"',
                                      'description: "Configure it"\n        remediationAction: inform'))
        self.assertIn('| `config-el-config` | inform |', md)
        self.assertIn('| `config-el-ready` | inform (always) |', md)

    def test_template_default_action(self):
        md = self.render(BASE.replace('        remediationAction: enforce\n        waitForOperator', '        waitForOperator'))
        self.assertIn('| `config-el-config` | inform |', md)

    def test_renders_compliance_type(self):
        md = self.render()
        self.assertIn('[el-configmap](converters/el-configmap.yaml)<br>[el-role](converters/el-role.yaml) (mustonlyhave)', md)
        self.assertIn('[operator](#operators) `el-operator`', md)

    def test_raw_converter_compliance_types(self):
        values = BASE.replace('        templateNames:\n          - name: el-configmap',
                              '        rawTemplate: true\n        complianceType: mustonlyhave\n'
                              '        templateNames:\n          - name: el-configmap')
        path = element_dir(self.root, values)
        (path / 'converters').mkdir()
        (path / 'converters' / 'el-configmap.yaml').write_text(
            "- complianceType: musthave\n- complianceType: mustnothave\n- complianceType: 'mustonlyhave'\n")
        md = doc_generator.generate(path)
        self.assertIn('[el-configmap](converters/el-configmap.yaml) (mustnothave, mustonlyhave)', md)
        # The entry's complianceType does not apply to raw converters.
        self.assertIn('[el-role](converters/el-role.yaml) | Example ConfigMap', md)

    def test_policy_anchor_unique(self):
        md = self.render(BASE.replace('- name: install', '- name: el').replace('policyRef: install', 'policyRef: el')
                         .replace('    enabled: false\n', '    enabled: false\n    toggles:\n      el: true\n', 1))
        self.assertIn('Policy [`el`](#policy-el)', md)
        self.assertIn('### Policy `el`', md)

    def test_operators_table(self):
        md = self.render()
        self.assertIn('| `install-el` | `el-operator` | `stable` | `redhat-operators` | `el-ns` | any |', md)
        md = self.fresh(BASE.replace('upgradeApproval: Automatic\n        versions: []',
                                            'upgradeApproval: None\n        versions: [el.v1, el.v2]'))
        self.assertIn('| `el.v1`<br>`el.v2`, None approval |', md)

    def test_no_toggles_or_settings_sections_when_absent(self):
        md = self.render()
        self.assertNotIn('## Toggles', md)
        self.assertNotIn('## Settings', md)

    def test_compliance_line(self):
        md = self.render(BASE.replace('    enabled: false\n', '    enabled: false\n    default:\n'
                                      '      standards: [NIST SP 800-53]\n      controls: [CM-2 Baseline]\n', 1))
        self.assertIn('Standards: NIST SP 800-53. Controls: CM-2 Baseline.', md)

    def test_operator_policies_list_rejected(self):
        values = "stack:\n  el:\n    operatorPolicies:\n      - name: x\n"
        with self.assertRaisesRegex(ValueError, 'operatorPolicies must be a map'):
            self.render(values)

    def test_unknown_policy_ref_rejected(self):
        with self.assertRaisesRegex(ValueError, "policyRef 'nope'"):
            self.render(BASE.replace('policyRef: config\n        remediationAction: enforce',
                                     'policyRef: nope\n        remediationAction: enforce'))

    def test_element_without_stack_key(self):
        self.assertIsNone(self.render("stack:\n  other: {enabled: true}\n"))


TOGGLED = BASE.replace("    enabled: false\n", """    enabled: false
    # Pick what runs here
    toggles:
      # Configure the operator
      config: false
      # The ready check
      el-ready: true
      # Turn on both a Policy and its same-name template
      install: true
""", 1)


class TogglesTest(Fixture):
    def test_section(self):
        md = self.render(TOGGLED)
        self.assertIn('## Toggles\n\nPick what runs here\n', md)
        self.assertIn('| `config` | Configure the operator | `false` | Policy [`config`](#policy-config) |', md)
        self.assertIn('| `el-ready` | The ready check | `true` | config `config-el-ready` (needs `config`) |', md)

    def test_same_name_template_covered_by_policy(self):
        md = self.render(TOGGLED.replace('- name: el-config', '- name: config'))
        self.assertIn('| `config` | Configure the operator | `false` | Policy [`config`](#policy-config) |', md)
        self.assertNotIn('(off, toggle `config`)', md)

    def test_policy_and_template_state(self):
        md = self.render(TOGGLED)
        self.assertIn('Off by default; turn on with toggle `config`.', md)
        md = self.fresh(TOGGLED.replace('el-ready: true', 'el-ready: false'))
        self.assertIn('`config-el-ready` (off, toggle `el-ready`)', md)

    def test_same_name_template_off_while_policy_on(self):
        md = self.render(BASE.replace('- name: el-ready', '- name: config').replace(
            'remediationAction: InformOnly', 'remediationAction: InformOnly\n        enabled: false', 1)
            .replace('      - name: config\n        enabled: true\n        policyRef: config',
                     '      - name: config\n        enabled: false\n        policyRef: config'))
        self.assertIn('`config-config` (off, toggle `config`)', md)

    def test_policy_without_templates_on(self):
        md = self.render(BASE.replace('    enabled: false\n', '    enabled: false\n    toggles:\n'
                                      '      el-config: false\n      el-ready: false\n', 1))
        self.assertIn('Renders nothing by default: every template is off.', md)

    def test_null_toggle_is_absent(self):
        md = self.render(BASE.replace('    enabled: false\n', '    enabled: false\n    toggles:\n      config: null\n', 1))
        self.assertNotIn('Off by default', md)

    def test_unmatched_toggle_rejected(self):
        with self.assertRaisesRegex(ValueError, "toggle 'ghost' matches no entry"):
            self.render(BASE.replace('    enabled: false\n', '    enabled: false\n    toggles:\n      ghost: true\n', 1))


SETTINGS = BASE.replace("    enabled: false\n", """    enabled: false
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
""", 1)


class SettingsTest(Fixture):
    def setUp(self):
        super().setUp()
        self.md = self.render(SETTINGS)

    def test_intro_and_fixed_line(self):
        self.assertIn('## Settings\n\nSite data\n\nUnder `stack.el.config`. Maps merge across layers; '
                      'lists are replaced whole.', self.md)

    def test_rows(self):
        for row in ['| `level` | Log level | `info` |',
                    '| `scanner` | Sizing |  |',  # all-scalar map expands
                    '| `scanner.min` |  | `2` |',
                    '| `auth` | Auth |  |',  # a commented child expands
                    '| `auth.provider` | Provider | `openshift` |',
                    '| `auth.groups` |  | `[a, b]` |',
                    '| `pools` | Pools keyed by name | 2 entries, below |',
                    '| `empty` |  | `{}` |',
                    '| `unset` |  | `null` |']:
            self.assertIn(row, self.md)

    def test_multi_line_string_in_details(self):
        self.assertIn('| `script` |  | below |', self.md)
        self.assertIn('<code>script</code> default</summary>\n\n```\nline1\nline2\n```', self.md)

    def test_long_default_in_details(self):
        self.assertIn('<details><summary><code>pools</code> default</summary>\n\n```yaml\nfirst:\n', self.md)
        self.assertIn('```\n\n</details>', self.md)


class MainTest(Fixture):
    def setUp(self):
        super().setUp()
        self.el = element_dir(self.root, BASE)

    def test_generate_then_check(self):
        self.assertEqual(self.run_main(), 0)
        self.assertTrue((self.el / 'README.md').read_text().startswith(doc_generator.HEADER))
        self.assertEqual(self.run_main('--check'), 0)

    def test_check_fails_when_missing_or_stale(self):
        self.assertEqual(self.run_main('--check'), 1)
        (self.el / 'README.md').write_text('stub\n')
        self.assertEqual(self.run_main('--check'), 1)
        self.assertEqual((self.el / 'README.md').read_text(), 'stub\n')

    def test_rerun_writes_nothing(self):
        self.run_main()
        self.assertFalse(doc_generator.write_if_changed(self.el / 'README.md', doc_generator.generate(self.el)))

    def test_single_element(self):
        self.assertEqual(self.run_main('--element', 'el'), 0)
        self.assertTrue((self.el / 'README.md').exists())
        self.assertEqual(self.run_main('--element', 'missing'), 1)

    def test_element_without_values_skipped(self):
        (self.root / 'empty').mkdir()
        self.assertEqual(self.run_main(), 0)
        self.assertFalse((self.root / 'empty' / 'README.md').exists())


if __name__ == '__main__':
    unittest.main()
