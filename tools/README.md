# PolicyStack Tools

This directory contains automation utilities for PolicyStack development and documentation. These tools streamline the creation of new policy elements and maintain synchronized documentation with your configuration.

## Table of Contents

- [Overview](#overview)
- [create-element.sh](#create-elementsh)
- [doc-generator.py](#doc-generatorpy)
  - [Purpose](#purpose)
  - [Architecture](#architecture)
  - [Usage](#usage)
  - [Comment Annotation System](#comment-annotation-system)
  - [Output Format](#output-format)
- [CI/CD Integration](#cicd-integration)
- [Best Practices](#best-practices)

## Overview

The PolicyStack tools provide:
- **Element Creation**: Automated scaffolding of new policy elements with proper structure
- **Documentation Generation**: Automatic extraction and formatting of inline documentation from YAML configurations
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

### Purpose

The documentation generator creates comprehensive markdown documentation from PolicyStack values.yaml files by:
- Parsing YAML with comment preservation at any nesting level
- Extracting `@description` and `@desc` annotations
- Generating structured documentation for all policy types
- Identifying configuration issues (orphaned policies)

### Architecture

```
DocumentationGenerator
├── YAMLLoader                   # Comment-aware YAML parser
│   ├── _parse_with_comments()   # Extracts comments at all levels
│   ├── load()                   # Returns data + comment structure
│   └── get_description()        # Retrieves descriptions by path
│
├── Element Processing
│   ├── generate_element_docs()  # Main doc generation per element
│   ├── _generate_policy_section()
│   ├── _generate_config_policy_section()
│   ├── _generate_operator_policy_section()
│   └── _generate_certificate_policy_section()
│
└── Utilities
    ├── to_camel_case()          # Name conversion
    ├── _find_sub_policies()     # Relationship mapping
    ├── _find_orphaned_policies()# Validation
    └── _calculate_statistics()  # Summary generation
```

### Usage

```bash
# Install the pinned dependencies
pip install -r tools/requirements.txt

# Generate documentation for all elements
python tools/doc-generator.py

# Custom stack directory
python tools/doc-generator.py --stack-dir ./custom-stack

# Single element documentation
python tools/doc-generator.py --element security-baseline
```

### Comment Annotation System

The generator supports hierarchical comment annotations using `@description:` or `@desc:` prefixes:

#### Basic Annotations

```yaml
# @description: Main policy for security compliance
security-policy:
  # @desc: Enable or disable this policy
  enabled: true
```

#### Nested Field Documentation

```yaml
configPolicies:
  - name: network-policy
    # @desc: Enforcement action when violations detected
    remediationAction: enforce
    
    # @description: Template configurations
    templateNames:
      # @desc: Ingress control template
      - name: ingress-rules
        complianceType: musthave
```

#### Array Item Documentation

```yaml
# @description: CSVs the OperatorPolicy may install
versions:
  # @desc: Currently installed
  - example-operator.v1.0.0
  # @desc: Next bump, remove v1.0.0 once every cluster runs it
  - example-operator.v1.0.1
```

#### Multi-line Annotations

Consecutive annotation lines are joined into one description. A blank line or plain comment ends the run.

```yaml
# @desc: Allow OSDs to be unevenly distributed. Set true on bare metal when the storage node
# @desc: count is not a multiple of three.
flexibleScaling: null
```

Parser tests: `python -m unittest tools/test_doc_generator.py`

### Output Format

The generator produces:

#### Individual Element Documentation (`stack/<element-name>/README.md`)

```markdown
# element-name - Policy Library Documentation

> Element description from Chart.yaml

Field reference: [policy-library](https://github.com/PolicyStack/PolicyStack-chart/tree/main/charts/policy-library)

*Generated: 2025-09-05 14:30:00*

## Component Configuration
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Component | `elementName` | Main component configuration |
| Enabled | `true` | Whether this component is enabled |

## Policies
### 📋 Policy: policy-name
> Policy description

[Detailed policy configuration tables...]

## PolicySets
[PolicySet configurations...]

## 📊 Summary
| Resource Type | Count |
| ------------- | ----- |
| Policies | 5 |
| Configuration Policies | 12 |
| **Total Resources** | **17** |
```

## CI/CD Integration

`docs-check.yml` runs the generator tests and `doc-generator.py --check` on pull requests, failing
when an element README is stale. Adding the `update-docs` label runs `docs-update.yml`, which
regenerates and commits `stack/*/README.md` to the PR branch. See [docs/workflows.md](../docs/workflows.md).

## Best Practices

### For doc-generator.py

1. **Comment Placement**: Place `@desc` comments immediately before the field they describe
2. **Description Quality**: Write descriptions that explain "why" not just "what"
3. **Consistency**: Use consistent terminology across descriptions
4. **Validation**: Run the generator locally before committing values changes
5. **Array Documentation**: Document array items when they have distinct purposes

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
