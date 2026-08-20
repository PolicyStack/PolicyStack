# PolicyStack Documentation Index

*Generated: 2026-08-20 21:44:36*

## Available Elements

- [advanced-cluster-management](./advanced-cluster-management.md)
- [advanced-cluster-security](./advanced-cluster-security.md)
- [ansible-automation-platform](./ansible-automation-platform.md)
- [cert-manager](./cert-manager.md)
- [cluster-observability](./cluster-observability.md)
- [dev-spaces](./dev-spaces.md)
- [developer-hub](./developer-hub.md)
- [external-secrets-operator](./external-secrets-operator.md)
- [gitops-dev](./gitops-dev.md)
- [infra-nodes](./infra-nodes.md)
- [kiali](./kiali.md)
- [local-storage](./local-storage.md)
- [loki](./loki.md)
- [lvm](./lvm.md)
- [machine-health-checks](./machine-health-checks.md)
- [manual-remediations](./manual-remediations.md)
- [master-nodes](./master-nodes.md)
- [metallb](./metallb.md)
- [mtv](./mtv.md)
- [nmstate](./nmstate.md)
- [node-feature-discovery](./node-feature-discovery.md)
- [node-maintenance](./node-maintenance.md)
- [openshift-compliance-operator](./openshift-compliance-operator.md)
- [openshift-data-foundation](./openshift-data-foundation.md)
- [openshift-dns](./openshift-dns.md)
- [openshift-gitops](./openshift-gitops.md)
- [openshift-image-registry](./openshift-image-registry.md)
- [openshift-logging](./openshift-logging.md)
- [openshift-pipelines](./openshift-pipelines.md)
- [openshift-upgrade](./openshift-upgrade.md)
- [openshift-virtualization](./openshift-virtualization.md)
- [opentelemetry](./opentelemetry.md)
- [quay](./quay.md)
- [servicemesh3-ambient](./servicemesh3-ambient.md)
- [servicemesh3operator](./servicemesh3operator.md)
- [storage-nodes](./storage-nodes.md)
- [tempo](./tempo.md)
- [trusted-artifact-signer](./trusted-artifact-signer.md)
- [user-workload-monitoring](./user-workload-monitoring.md)
- [worker-nodes](./worker-nodes.md)
- [workload-partitioning](./workload-partitioning.md)

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