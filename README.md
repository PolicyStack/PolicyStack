# PolicyStack
PolicyStack is a GitOps implementation utilizing ACM (Advanced Cluster Management) policies to propagate configuration. The purpose is to create a "stack" of policies to apply to clusters. It allows for fine-grained configuration changes on various different bases (per-cluster, per-environment, etc). The included helm chart allows for `elements` (defined as a single helm chart inside the stack) in this "policy stack" to be modified at any point (unlike an actual stack, each element can be modified and enabled/disabled at any point). It also allows for rapid development and integration of new configurations due to the simple nature of helm.
## Tools
### How to create new configuration
1. Run the `tools/create-element.sh` script. It will accept two user-inputs. First is the name of the element you would like to create. Only alphanumeric characters and dashes (-) are accepted. Second is a description. This will be used for the chart description.
2. This will create a directory under the `stack/` dir with the configured helm chart.
3. Update the `values.yaml` w/ everything that needs to be deployed. This includes updating the `converters` dir.

> [!WARNING]  
> The max characters for a policy is 63. This consists of `<policy-namespace>.<policy-name>`. The `<policy-name>` includes the `<name in values>`-`<element-name>`-`<cluster-name>`. Make sure this is taken into account when creating policy names in a values file. Try to keep it as short as possible.

### Documentation

#### Overview
PolicyStack includes an automated documentation generator that creates comprehensive markdown documentation from your helm chart values files. This tool parses special comment annotations in `values.yaml` files to generate detailed, human-readable documentation for each element in your policy stack.

#### Why?

Managing complex policy configurations across multiple clusters requires clear documentation, but manually maintaining docs alongside code is error-prone and time-consuming. This generator solves several problems:

1. **Single Source of Truth**: Documentation lives directly in your values files as comments, ensuring it stays synchronized with actual configurations
2. **Consistency**: Generates uniform documentation structure across all stack elements
3. **Completeness**: Automatically documents all enabled policies, configurations, operators, and certificates with their relationships
4. **Validation**: Identifies orphaned sub-policies that reference disabled or non-existent parent policies
5. **Compliance Tracking**: Clearly shows which security controls, standards, and categories each policy addresses

#### Usage

##### Generate documentation for all elements:
```bash
python doc-generator.py
```

##### Specify custom directories:
```bash
python doc-generator.py --stack-dir ./stack --output-dir ./docs
```

##### Generate documentation for a specific element:
```bash
python doc-generator.py --element security-config
```

#### Comment Annotation Syntax

Add descriptions to any field in your values.yaml using `@description:` or `@desc:` comments placed immediately before the field:

```yaml
stack:
  # @description: Main security configuration for baseline controls
  securityConfig:
    # @desc: Enable or disable this entire configuration set
    enable: true
    
    policies:
      # @description: Network segmentation and traffic control policy
      - name: network-security
        enabled: true
        # @desc: Automatically remediate violations (inform/enforce)
        remediationAction: enforce
        
        # @description: Approved operator versions for controlled upgrades
        versions:
          # @desc: Initial stable release
          - v1.0.0
          # @desc: Security patch with CVE fixes
          - v1.0.1
```

#### Generated Documentation Structure

The generator creates:
- **Element Overview**: Chart description and metadata
- **Component Configuration**: High-level settings and defaults
- **Policies Section**: Detailed documentation for each policy including:
  - Compliance metadata (categories, controls, standards)
  - Associated sub-policies (configurations, operators, certificates)
  - Severity and remediation actions
- **PolicySets**: Grouped policy collections
- **Warnings**: Identifies orphaned policies and configuration issues
- **Summary Statistics**: Resource counts and totals

#### Output

Documentation is generated as markdown files in the `docs/` directory (or specified output directory):
- Individual `.md` file for each element
- `README.md` index file listing all documented elements
- Full comment notation guide for contributors

This automation ensures your PolicyStack documentation remains accurate, comprehensive, and aligned with your actual configurations, making it easier for teams to understand, audit, and maintain your cluster policies.

## Values File Structure for GitOps

This document explains how values files are organized and merged in this GitOps implementation using Argo CD ApplicationSets.

### Prerequisite Knowledge
Since the ignoreMissingValuesFiles is enabled, not every file needs to be created. This means when you create a new application in apps/, you only need to modify:
1. The helm values.yaml
2. Any specific environments/datacenters/clusters that need the app to be configured differently or enabled/disabled

It is best-practice to set `enabled: false` in the helm values.yaml and enable on a per-environment basis.

## Prereqs
There are several prerequisite items to install on an OpenShift cluster before this can be used.
1. ACM
2. OpenShift GitOps
3. Ensure the ArgoCD Application Controller SA has cluster-admin
4. `./gitops-prereq` please apply this directory. It will create a GitOpsCluster for pulling in the ACM clusters to GitOps.

## Installation
The installation is configured via the `./appset` helm chart.

```
1. Configure `appset` values.yaml, see README for details
2. helm install appset ./appset -f ./appset/values.yaml -f values.yaml
```

## Overview

Our GitOps implementation uses a hierarchical approach to values files, allowing for:
- Default values at the chart level
- Global values across all applications
- Environment-specific configurations
- Datacenter-specific configurations
- Dynamically generated configurations based on need
- Cluster-specific overrides

This structure enables consistent configuration across similar environments while allowing for targeted customization where needed.  
This structure also assumes there is a max of one ACM Hub cluster per datacenter.

## Label Structure
### Config Labels
Config labels are used to specify new values files to add to certain clusters. They use the scheme `config.example.com/type.priority=value`
`config.example.com/`: used as a prefix to denote a config label.
`type`: this is the actual label that will be pulled in to the values/ directory inside the git repo.
`priority`: This is the priority number associated with the new label. 1 is the lowest priority and there is no upper limit. A value specified in a lower priority will be overwritten by a value specified in a higher priority.
`value`: This is the actual value associated with the label.

### Git Labels
Currently, there is a single label for any kind of git configuration.
`git.example.com/revision`: This is used to specify the revision name for the cluster. This could be a tag or branch name.
***NOTE***: branch names/tags cannot have forward slashes in them. Labels in Kubernetes cannot contain forward slashes in label values. 

This configuration allows for addition of new values without the need to change the ApplicationSet. Now, if `tenant` values are needed, then `config.example.com/tenant.3=tenant1` can be added to a cluster. This would allow `../../values/tenant/tenant1.yaml` to be created.
***NOTE***: Not every values file needs to be created. See [here](#missing-values-files) for more information.
## Values File Hierarchy

Values files are loaded in a specific, but dynamic, order, with later files overriding values from earlier ones. :

1. `values.yaml` - Chart's default values file
2. `../../values.yaml` - Global values across all applications
3. `../../values/environments/<environment>.yaml` - Environment-specific values
4. `../../values/datacenters/<datacenter>.yaml` - Datacenter-specific values
5. `../../values/<type>/<value>.yaml` - Dynamically generated values based on the above mentioned config labels. See [here](#config-labels). Priority 3 will come after datacenter and 4 will have more priority over 3
6. Cluster-specific values (depends on cluster type):
   - For local-clusters (ACM hub):
     - `../../values/acm/acm-<datacenter>.yaml`
     - `../../values/clusters/acm-<datacenter>.yaml`
   - For managed clusters:
     - `../../values/clusters/<cluster-name>.yaml`

## Directory Structure

```
repository/
├── apps/
│   └── sample-chart/
│       └── values.yaml       # (1) Chart default values
├── values.yaml               # (2) Global values
└── values/
    ├── environments/
    │   ├── dev.yaml # (3) Environment-specific values for dev
    │   ├── test.yaml
    │   ├── prod.yaml
    │   └── ...
    ├── datacenters/
    │   ├── dc1.yaml # (4) Datacenter-specific values for dc1
    │   ├── dc2.yaml
    |   └── ...
    ├── <type>/
    │   ├── <value>.yaml # (4) Dynamic values specified by labels. 
    ├── acm/
    │   ├── acm-dc1.yaml # (5a) ACM specific values for dc1
    │   └── acm-dc2.yaml
    └── clusters/
        ├── acm-dc1.yaml # (5b) Cluster-specific values for acm-dc1
        ├── cluster1.yaml # (5c) Cluster-specific values for cluster1
        └── cluster2.yaml
```

## How Values Files Are Merged

1. The chart's local `values.yaml` provides default values
2. Global values from `../../values.yaml` override chart defaults
3. Environment values override global values
4. Datacenter values override environment values
5. Dynamically assigned vars from the config.exam<span>pl</span>.com/type.priority label will overwrite any lower privileged values
5. Cluster-specific values provide the final overrides

Values are merged using Helm's deep merging behavior, where nested structures are combined rather than replaced completely.

## Required Labels
Currently, the only "required" label is the revision label. This is fairly limiting, but will be functional and allow for per-cluster configuration.

1. `git.example.com/revision=<revision>`: This would be a revision in git for the cluster to pull configuration from. This could be a tag or branch

> [!WARNING]  
> The revision must be a normalized value as this will be a K8s label. No forward slashes (/), quotes ("), etc.

## Recommended Labels
Here are some recommended labels that would allow you to take advantage of multiple environments.
1. `config.example.com/envioronment.1=<environment>`: This would be prod/nonprod/sbx or any other custom environment
2. `config.example.com/datacenter.2=<datacenter>`: This would be nj/mtc or custom
3. `config.example.com/<type>.<priority>=<value>`: Dynamically generated label - ***OPTIONAL***

### Missing Values Files

The ApplicationSet is configured with `ignoreMissingValueFiles: true`, which means if a referenced values file is missing, it will be silently skipped rather than causing an error.

This allows for a flexible structure where not every chart needs values files at every level.

## Operator Upgrades
To update an operator, you must add a new array entry under the operatorPolicy versions array.

For instance, take this example.
```yaml
groupSyncOperator:
  operatorPolicies:
    - name: group-sync
      enabled: true
      description: "Operator policy for group sync"
      displayName: "Group Sync Operator"
      policyRef: "group-sync-operator-policy"
      severity: medium
      remediationAction: enforce
      complianceType: musthave
      namespace: group-sync-operator
      operatorGroup:
          name: group-sync-operator
          namespace: group-sync-operator
          targetNamespaces:
            - group-sync-operator
      subscription:
          channel: alpha
          name: group-sync-operator
          source: community-operator-index
          sourceNamespace: openshift-marketplace
          startingCSV: group-sync-operator.v0.0.31
          config:
            tolerations:
              - key: node-role.kubernetes.io/infra
                value: ""
                effect: NoSchedule
              - key: node-role.kubernetes.io/infra
                value: ""
                effect: NoExecute
      upgradeApproval: Automatic
      versions:
        - group-sync-operator.v0.0.32
```

The `group-sync-operator.v0.0.32` version is the only approved version. ACM will approve the upgrade **only** if the version is in this list. You can also set a maximum version and it will install all versions up to that version.

