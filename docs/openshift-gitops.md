# openshift-gitops - Policy Library Documentation

> OpenShift GitOps operator and the platform ArgoCD instance

*Generated: 2026-08-20 21:44:36*

## Component Configuration

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Component | `openshiftGitops` | OpenShift GitOps operator and the platform ArgoCD instance |
| Enabled | `False` | Master control to enable/disable all policies in this element |

## Default Policy Metadata

Default policy metadata applied unless overridden per-policy

| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | CM Configuration Management | Default category classifications |
| Controls | CM-2 Baseline Configuration | Default control mappings |
| Standards | NIST SP 800-53 | Default compliance standards |

## Sub-Feature Toggles

Override an entry's `enabled` by name. Being a map, these merge cleanly through the values cascade, so a per-cluster override stays one line.

| Toggle | Default | Description |
| ------ | ------- | ----------- |
| `instance` | `True` | Deploy the platform ArgoCD instance. Off where you only want the operator. |
| `ca-bundle` | `True` | Give ArgoCD the cluster's trusted CA bundle |
| `link` | `True` | Publish a console link to ArgoCD |

## Configuration

Values intended to be overridden per environment, datacenter, or cluster.

| Key | Default | Description |
| --- | ------- | ----------- |
| `namespace` | `openshift-gitops` | Namespace the platform ArgoCD instance runs in |
| `disableAdmin` | `False` | Disable the built-in admin account and rely on OpenShift OAuth |
| `monitoring` | `False` | Expose ArgoCD metrics to the monitoring stack |
| `kustomizeBuildOptions` | `--enable-helm --helm-command /usr/local/bin/helm` | Kustomize flags. Helm support is on; PolicyGenerator plugin flags are deliberately absent - PolicyStack does not use that plugin. |
| `rbacPolicies` | `(list)` | ArgoCD RBAC, one CSV line per entry |
| `ha` | `(dict)` |  |
| `serverAutoscale` | `(dict)` |  |
| `resources` | `(dict)` | Per-component resource requests and limits |

## Policies

### 📋 Policy: install
> Install and manage the OpenShift GitOps operator

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `install-<release>` | Full policy name including release |
| Namespace | `<namespace>` | Policy namespace |
| Enabled | `True` | Whether this policy is templated |
| Severity | `medium` | Policy severity level |
| Remediation | `enforce` | Action when policy is violated |

#### Compliance Metadata
| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | CM Configuration Management (default) | Category classifications |
| Controls | CM-2 Baseline Configuration (default) | Control mappings |
| Standards | NIST SP 800-53 (default) | Compliance standards |

#### Associated Sub-Policies

##### Operator Policies

###### 🔧 Operator: gitops
> OpenShift GitOps operator

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `install-gitops` | Operator policy identifier |
| Namespace | `openshift-gitops-operator` | Target namespace for operator |
| Display Name | `Red Hat OpenShift GitOps` | Display name for operator |
| Compliance Type | `musthave` | Compliance requirement |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |
| Upgrade Approval | `Automatic` | Upgrade approval strategy |

**Subscription Details:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `openshift-gitops-operator` | Operator package name |
| Channel | `latest` | Update channel |
| Source | `redhat-operators` | Catalog source |
| Source Namespace | `openshift-marketplace` | Catalog namespace |


---

### 📋 Policy: instance
> Platform ArgoCD instance

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `instance-<release>` | Full policy name including release |
| Namespace | `<namespace>` | Policy namespace |
| Enabled | `True` | Whether this policy is templated |
| Severity | `medium` | Policy severity level |
| Remediation | `enforce` | Action when policy is violated |

#### Dependencies

This policy stays `Pending` until every target below reports the listed compliance state.

| Resolves To | Kind | Awaited State |
| ----------- | ---- | ------------- |
| `install-<release>` | `Policy` | `Compliant` |

#### Compliance Metadata
| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | CM Configuration Management (default) | Category classifications |
| Controls | CM-2 Baseline Configuration (default) | Control mappings |
| Standards | NIST SP 800-53 (default) | Compliance standards |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: ca-bundle
> Trusted CA bundle for ArgoCD

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `instance-ca-bundle` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/gitops-ca-bundle.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `argoName` | `openshift-gitops` | Parameter value |
| `caBundleName` | `gitops-cluster-ca-bundle` | Parameter value |
| `consoleText` | `Red Hat OpenShift GitOps` | Parameter value |
| `imageUrl` | `https://upload.wikimedia.org/wikipedia/commons/3/3a/OpenShift-LogoType.svg` | Parameter value |

###### ⚙️ Config: argocd
> ArgoCD instance

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `instance-argocd` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `medium` | Severity level |

**Gating:**

- Waits for operator `gitops` to reach CSV phase `Succeeded`
- Reports compliant while waiting (`ignorePending`)

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/argocd-instance.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `argoName` | `openshift-gitops` | Parameter value |
| `caBundleName` | `gitops-cluster-ca-bundle` | Parameter value |
| `consoleText` | `Red Hat OpenShift GitOps` | Parameter value |
| `imageUrl` | `https://upload.wikimedia.org/wikipedia/commons/3/3a/OpenShift-LogoType.svg` | Parameter value |


---

### 📋 Policy: link
> Console link to ArgoCD

| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `link-<release>` | Full policy name including release |
| Namespace | `<namespace>` | Policy namespace |
| Enabled | `True` | Whether this policy is templated |
| Severity | `low` | Policy severity level |
| Remediation | `enforce` | Action when policy is violated |

#### Dependencies

This policy stays `Pending` until every target below reports the listed compliance state.

| Resolves To | Kind | Awaited State |
| ----------- | ---- | ------------- |
| `instance-<release>` | `Policy` | `Compliant` |

#### Compliance Metadata
| Type | Values | Description |
| ---- | ------ | ----------- |
| Categories | CM Configuration Management (default) | Category classifications |
| Controls | CM-2 Baseline Configuration (default) | Control mappings |
| Standards | NIST SP 800-53 (default) | Compliance standards |

#### Associated Sub-Policies

##### Configuration Policies

###### ⚙️ Config: console
> ArgoCD console link

**Basic Configuration:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| Name | `link-console` | Configuration policy identifier |
| Compliance Type | `musthave` | Compliance requirement type |
| Remediation | `enforce` | Remediation action |
| Severity | `low` | Severity level |

**Templates:**
| Template File | Compliance Type | Description |
| ------------- | --------------- | ----------- |
| `converters/gitops-console-link.yaml` | inherited | Template configuration |

**Template Parameters:**
| Parameter | Value | Description |
| --------- | ----- | ----------- |
| `argoName` | `openshift-gitops` | Parameter value |
| `caBundleName` | `gitops-cluster-ca-bundle` | Parameter value |
| `consoleText` | `Red Hat OpenShift GitOps` | Parameter value |
| `imageUrl` | `https://upload.wikimedia.org/wikipedia/commons/3/3a/OpenShift-LogoType.svg` | Parameter value |


---

## 📊 Summary

| Resource Type | Count |
| ------------- | ----- |
| Policies | 3 |
| Configuration Policies | 3 |
| Operator Policies | 1 |
| Certificate Policies | 0 |
| PolicySets | 0 |
| **Total Resources** | **7** |