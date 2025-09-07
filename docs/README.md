# PolicyStack Documentation Index

*Generated: 2025-09-07 21:37:35*

## Available Elements

- [test-element](./test-element.md)

## Comment Notation Guide

Use special comment notation in values.yaml files to add descriptions at any level:

### Basic Usage

```yaml
# @description: This policy enforces security standards
security-policy:
  enabled: true
```

### Nested Field Descriptions

```yaml
configPolicies:
  - name: example-config
    # @desc: Whether to actually apply this configuration
    enabled: true
    
    # @description: Individual template configurations
    templateNames:
      # @desc: Network policy template for namespace isolation
      - name: network-policy
        complianceType: musthave
      
      # @desc: RBAC template for role bindings
      - name: rbac-config
        complianceType: musthave
    
    # @description: Template parameters with specific values
    templateParameters:
      # @desc: The namespace to apply policies to
      targetNamespace: production
      
      # @desc: Severity level for alerts (low/medium/high/critical)
      alertLevel: high
```

### Array Item Descriptions

```yaml
operatorPolicies:
  # @description: GitOps operator for continuous deployment
  - name: openshift-gitops
    enabled: true
    
    # @desc: Which approved versions can be installed
    versions:
      # @desc: Initial stable release
      - gitops-operator.v1.5.0
      # @desc: Security patch release
      - gitops-operator.v1.5.1
      # @desc: Feature update with performance improvements
      - gitops-operator.v1.6.0
```

## Notes

- Place `@description:` or `@desc:` comments on the line immediately before the field
- Descriptions work at any nesting level
- Array items can be documented by placing the comment before the item
- Both `@description:` and `@desc:` are supported (they're equivalent)
