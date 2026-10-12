# PolicyStack Tools

This directory contains automation utilities for PolicyStack development and documentation. These tools streamline the creation of new policy elements and maintain synchronized documentation with your configuration.

## Table of Contents

- [Overview](#overview)
- [create-element.sh](#create-elementsh)
  - [Purpose](#purpose)
  - [Usage](#usage)
  - [Configuration](#configuration)
  - [Implementation Details](#implementation-details)
- [doc-generator.py](#doc-generatorpy)
  - [Purpose](#purpose-1)
  - [Architecture](#architecture)
  - [Usage](#usage-1)
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

### Purpose

The element creation script automates the scaffolding of new PolicyStack elements by:
- Copying a template chart structure
- Replacing placeholder values with user-provided names
- Converting naming conventions (kebab-case to camelCase)
- Ensuring consistent element structure across the stack

### Usage

```bash
# Basic usage
./tools/create-element.sh

# The script will prompt for:
# 1. Chart name (alphanumeric and dashes only)
# 2. Chart description
```

#### Example

```bash
$ ./tools/create-element.sh
=== Element Chart Setup Script ===

Enter the chart name (only letters, numbers, and dashes allowed): security-baseline
Enter the chart description: Baseline security controls for cluster hardening

Creating new Element chart...
- Name: security-baseline
- Description: Baseline security controls for cluster hardening
- CamelCase name: securityBaseline
- Destination: ./stack/security-baseline

✓ Element chart successfully created!
Location: ./stack/security-baseline
```

### Configuration

The script uses configurable variables at the top of the file:

```bash
# Configuration
SOURCE_DIR="./sample-element"    # Template source directory
DESTINATION_BASE="./stack"       # Where elements are created
```

### Implementation Details

#### Name Transformation

The script performs automatic name transformation for values.yaml compatibility:

```bash
# Input: security-baseline
# Output in values.yaml: securityBaseline
```

This transformation ensures proper helm value referencing in the PolicyStack structure.

#### File Modifications

1. **Chart.yaml**: Updates name and description fields
2. **values.yaml**: Replaces the sample's stack key (the camelCase form of its chart name, `myTest`) with the new camelCase name
3. **Directory Structure**: Preserves the complete template structure including:
   - `templates/` directory with policy.yaml
   - `converters/` directory for manifest templates
   - `.gitignore` for helm dependencies

#### Error Handling

- Validates input names (alphanumeric + dashes only)
- Checks for existing directories with overwrite confirmation
- Verifies source template existence
- Provides colored output for better UX

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

### For create-element.sh

1. **Naming Conventions**: Use kebab-case for element names (e.g., `security-baseline`, not `security_baseline`)
2. **Descriptions**: Write clear, concise descriptions that explain the element's purpose
3. **Template Maintenance**: Keep `sample-element` minimal but complete
4. **Version Control**: Commit new elements immediately after creation

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
