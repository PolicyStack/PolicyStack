# PolicyStack Tools

This directory contains automation utilities for PolicyStack development and documentation. These tools streamline the creation of new policy elements and maintain synchronized documentation with your configuration.

## Table of Contents

- [Overview](#overview)
- [create-element.sh](#create-elementsh)
- [doc-generator.py](#doc-generatorpy)
- [migrate-4.0.py](#migrate-40py)
- [policystack/](#policystack)
- [CI/CD Integration](#cicd-integration)
- [Best Practices](#best-practices)

## Overview

The PolicyStack tools provide:
- **Element Creation**: Automated scaffolding of new policy elements with proper structure
- **Documentation Generation**: Element READMEs generated from `Chart.yaml`, `values.yaml` and `converters/`
- **CI/CD Support**: GitHub Actions workflows for documentation validation and auto-generation

## create-element.sh

```bash
./tools/create-element.sh <name> "<description>"
```

Copies `tools/sample-element/` to `stack/<name>/`, replacing `my-element` with the name and `myElement`
with its camelCase form, sets the description and generates the README. It prompts for missing
arguments and refuses a name that is not lowercase kebab-case, is longer than 17 characters
(POLICY001), or equals, extends or prefixes an existing element's name (POLICY091). The copy keeps
the sample's policy-library dependency (`file://../../lib/policy-library`, floor `^4.0.0`), which
resolves from `stack/<name>/` as it does from `tools/sample-element/`. The new element passes the
validator and prek as is, marked `policystk.io/stability: preview`. In upstream PolicyStack (where
`.release-please/` exists) it also copies the answers template `{{_copier_conf.answers_file}}.jinja`
into the element, so adopters can copy it from the next release. In an adopter's copy it doesn't,
and the element stays local. See
[Workflow](https://policystk.io/develop/elements/#workflow).

## doc-generator.py

```bash
pip install -r tools/requirements.txt
python tools/doc-generator.py                  # all elements
python tools/doc-generator.py --element acs    # one element
python tools/doc-generator.py --check          # exit 1 when a README is missing or stale
python -m unittest tools/test_doc_generator.py
```

Writes `stack/<element>/README.md` from `Chart.yaml`, `values.yaml` and `converters/`: the
stability from the `policystk.io/stability` annotation, the values key, optional pieces (`enabled: false` entries), settings, operators, and each Policy with its
templates, actions, dependencies and converters.

The comment lines directly above a key under `config:`, or above `config:` itself, are its
description. A blank line ends the block, so a maintainer note separated by one is not rendered.
Other comments are never rendered. A stability other than `stable` or `preview` is an error; an
element without the annotation gets no stability line. See [Element reference](https://policystk.io/develop/elements/#element-reference).

It fails on a policy-library 3.x `values.yaml` (`policies` list, `configPolicies`); run
`tools/migrate-4.0.py` first.

## migrate-4.0.py

```bash
uv run tools/migrate-4.0.py                    # the whole repo
uv run tools/migrate-4.0.py stack/<el> values  # only these paths
```

Converts elements and values layers from policy-library 3.x to 4.0. Idempotent: skips what is
already 4.0. Prints what it could not convert; review the result with `git diff` and Render Diff.
Without uv, `pip install 'ruamel.yaml>=0.18'` first.

## policystack/

The launcher. It creates a PolicyStack repository, adds elements to it, and updates it to a newer
release. It calls copier's Python API with the copier version `uv.lock` pins, so nothing but mise
and git 2.43 or later is needed. Run `install` from a clone of upstream PolicyStack, and the other
commands from your repository's root:

```bash
mise run ps -- install ~/fleet                  # pick release, elements, example values files
mise run ps -- install --to v1.4.0 --elements odf,loki --examples values/clusters/prod-east-1.yaml ~/fleet
mise run ps -- add loki                         # copy elements at the framework's release
mise run update                                 # ps update, then validate
mise run ps -- update --to v1.5.0 odf           # update only odf; refused if another element must move too
```

Commands, exit codes, the update branch and the `upgrades/` and `blocked.yaml` formats:
[Launcher reference](https://policystk.io/reference/launcher/).

Tests run in upstream only (`tests/` is not copied). `test_flows.py` runs install, add and update
end to end through copier against a synthetic upstream, which takes about a minute:

```bash
mise x -- uv run --project tools/policystack pytest tools/policystack/tests
```

## CI/CD Integration

Upstream's `upstream.yml` runs the generator tests, and the launcher's tests and `uv lock --check`. The `element-readmes` prek hook, run by `ci.yml`,
regenerates `stack/*/README.md` and fails when one is stale. See
[Hooks](https://policystk.io/reference/ci/#hooks).

## Best Practices

### For doc-generator.py

1. **Comment Placement**: Put a description directly above the setting it describes
2. **Description Quality**: Write descriptions that explain "why" not just "what"
3. **Consistency**: Use consistent terminology across descriptions
4. **Validation**: Run the generator locally before committing values changes

### General

1. **Automation**: Use CI/CD workflows to maintain documentation consistency
2. **Review Process**: Include documentation review in PR reviews
3. **Versioning**: Document breaking changes in element configurations
4. **Testing**: Test element creation and documentation generation in development branches

## Contributing

When adding new features to these tools:

1. **Document the Feature**: Update this README with usage examples
2. **Add Tests**: Include test cases in your PR description
3. **Maintain Backwards Compatibility**: Don't break existing workflows
4. **Update CI/CD**: Adjust workflows if needed
5. **Consider Extensibility**: Design features to be easily extended
