# PolicyStack Tools

This directory contains automation utilities for PolicyStack development and documentation. These tools streamline the creation of new policy elements and maintain synchronized documentation with your configuration.

## Table of Contents

- [Overview](#overview)
- [create-element.sh](#create-elementsh)
- [doc-generator.py](#doc-generatorpy)
- [CI/CD Integration](#cicd-integration)
- [Best Practices](#best-practices)

## Overview

The PolicyStack tools provide:
- **Element Creation**: Automated scaffolding of new policy elements with proper structure
- **Documentation Generation**: Element READMEs generated from `Chart.yaml` and `values.yaml`
- **CI/CD Support**: GitHub Actions workflows for documentation validation and auto-generation

## create-element.sh

```bash
./tools/create-element.sh <name> "<description>"
```

Copies `tools/sample-element/` to `stack/<name>/`, replacing `my-element` with the name and `myElement`
with its camelCase form, and sets the description. It prompts for missing arguments and refuses a
name that is not lowercase kebab-case or already exists. The new element passes the validator as is.
See [Creating an element](../docs/elements.md#creating-an-element).

## doc-generator.py

```bash
pip install -r tools/requirements.txt
python tools/doc-generator.py                  # all elements
python tools/doc-generator.py --element acs    # one element
python tools/doc-generator.py --check          # exit 1 when a README is missing or stale
python -m unittest tools/test_doc_generator.py
```

Writes `stack/<element>/README.md` from `Chart.yaml` and `values.yaml`: the values key, toggles,
settings, operators, and each Policy with its templates, actions, dependencies and converters.

The comment lines directly above a key under `toggles:` or `config:`, or above those two keys, are
its description. A blank line ends the block, so a maintainer note separated by one is not
rendered. Other comments are never rendered. See
[Element reference](../docs/elements.md#element-reference).

It fails on a toggle that matches no entry and on a `policyRef` that names no policy.

## CI/CD Integration

`docs-check.yml` runs the generator tests and `doc-generator.py --check` on pull requests, failing
when an element README is stale. Adding the `update-docs` label runs `docs-update.yml`, which
regenerates and commits `stack/*/README.md` to the PR branch. See [docs/workflows.md](../docs/workflows.md).

## Best Practices

### For doc-generator.py

1. **Comment Placement**: Put a description directly above the toggle or setting it describes
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
