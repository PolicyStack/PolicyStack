# Compliance

The [`compliance`](https://github.com/PolicyStack/PolicyStack/tree/main/stack/compliance) element installs the Compliance Operator, scans the cluster against a benchmark (DISA STIG by default), and reports failed checks in Governance: one Policy per cluster, `findings-compliance-<cluster>`, with one template per severity that turns NonCompliant and names the failed results ([Reading Governance](#reading-governance)). Optionally it applies the operator's fixes.

!!! note "Two meanings of compliance"
    Every ACM policy has a compliance state (Compliant, NonCompliant, Pending; see [ACM policies in brief](../concepts/acm.md)). This element turns benchmark scan results into such states.

## Turn it on

```yaml
# values/environments/prod.yaml
stack:
  compliance:
    enabled: true
```

Defaults ([README](https://github.com/PolicyStack/PolicyStack/blob/main/stack/compliance/README.md#settings)):

| Key under `stack.compliance.config` | Default |
|---|---|
| `profiles` | `[ocp4-stig, ocp4-stig-node, rhcos4-stig]` |
| `binding` | `stig`, the ScanSettingBinding's name, whatever the profiles |
| `rawResultStorage` | On, default StorageClass |
| `waivers`, `remediations` | Empty |

Without a default StorageClass, set `rawResultStorage.storageClass` or `rawResultStorage.enabled: false`. Otherwise the raw-results PVC stays Pending and scans never finish.

Scans run on the operator's own `default` ScanSetting schedule: daily, on master and worker nodes. Remediation is off ([Remediations](#remediations)).

!!! warning "Hosted control planes"
    Red Hat ships the operator on HCP as Technology Preview, with `PLATFORM=HyperShift` set in the Subscription's `config` and a worker `nodeSelector`. This element sets neither, so a hosted cluster (HCP guest) can't scan and shows NonCompliant in `findings-scan-state`. Leave the element off there.

## How a scan works

The element writes the rows marked Element, plus `spec.apply` on declared remediations ([Remediations](#remediations)); the operator does the rest.

| Object | Created by | What it is |
|---|---|---|
| Profile | Operator content | A benchmark, shipped in the operator's content. Platform profiles (`ocp4-stig`) check the cluster's API objects; node profiles (`ocp4-stig-node`, `rhcos4-stig`) check each node |
| ScanSetting `default` | Operator; the element patches storage only | How and when to scan. Its schedule and roles stay the operator's |
| Role, RoleBinding `compliance-operator-agg-patch-*` | Element | Lets the operator's remediation aggregator update ComplianceRemediations |
| ScanSettingBinding (`ssb`) `<binding>` | Element | Binds the profiles to the setting. Creating it starts the first scan |
| ComplianceSuite `<binding>` | Operator | Groups the binding's scans |
| ComplianceScan | Operator | One per profile, and one per role for node profiles: `ocp4-stig`, `ocp4-stig-node-master`, `rhcos4-stig-worker` |
| ComplianceCheckResult (`ccr`) | Operator | One rule's result in one scan, named `<scan>-<rule>`. Status `PASS`, `FAIL`, `ERROR`, `MANUAL`, `INCONSISTENT` and others. The rule's own name is on annotation `compliance.openshift.io/rule` |
| ComplianceRemediation | Operator | The fix for a failed result that has one, named like the result |

Each rule has three names:

| | Example |
|---|---|
| Rule (annotation `compliance.openshift.io/rule`) | `sysctl-kernel-dmesg-restrict` |
| Results | `rhcos4-stig-master-sysctl-kernel-dmesg-restrict`, `rhcos4-stig-worker-sysctl-kernel-dmesg-restrict` |
| Remediations | The same names as the results |

[Waivers](#waivers) take the rule name. [Remediations](#remediations) take the remediation name. Governance shows result names.

## What it creates

`<cluster>` is the ManagedCluster name, or `hubName` on the hub ([Rendered names](../concepts/elements.md#rendered-names)). `findings` only reports and never changes the cluster; the other Policies enforce, apart from their `-status` and `-state` checks.

| Policy | Templates | Waits for |
|---|---|---|
| `install-compliance-<cluster>` | `install-compliance-ns`, `install-compliance` (OperatorPolicy, package `compliance-operator`, channel `stable`), `install-compliance-status` ([The install Policy](../concepts/elements.md#the-install-policy)) | |
| `scan-compliance-<cluster>` | `scan-setting`: ScanSetting, the RBAC the operator's remediation aggregator needs, and the ScanSettingBinding | `install-compliance-<cluster>` Compliant |
| `findings-compliance-<cluster>` | `findings-scan-state`, `-high`, `-medium`, `-low`, `-waived`, `-drift` | `scan-compliance-<cluster>` Compliant |
| `remediate-compliance-<cluster>` | `remediate-remediations`, `remediate-remediation-state`. Only with `policies.remediate.enabled: true` | `scan-compliance-<cluster>` Compliant |

So `findings` shows Pending right after you enable the element, until the operator is installed and the binding is written. Then `findings-scan-state` is NonCompliant, because no results exist yet, until the first scan finishes.

## Reading Governance

In the console: **Governance > Policies > `findings-compliance-<cluster>`**, then a template.

| Template | Policy | NonCompliant means |
|---|---|---|
| `findings-scan-state` | `findings-compliance-<cluster>` | No suite or no results yet, a configured Profile doesn't exist, or a scan ended `ERROR`, `NOT-AVAILABLE` or `NOT-APPLICABLE` or hasn't started in 48 hours (often a cordoned node). `oc get compliancescan -n openshift-compliance` shows which |
| `findings-high`, `-medium`, `-low` | `findings-compliance-<cluster>` | Results with that severity that failed and aren't waived. `-low` takes every severity other than high and medium, unset included. See below for `ERROR` and `INCONSISTENT` |
| `findings-drift` | `findings-compliance-<cluster>` | A ComplianceRemediation is applied (`spec.apply: true`) but not declared `true` in `config.remediations` |
| `findings-waived` | `findings-compliance-<cluster>` | Never. Lists each result of a waived rule with its current status |
| `remediate-remediation-state` | `remediate-compliance-<cluster>` | A declared remediation hasn't reached its state. The message lists each as `<name>=<applicationState>`, `missing` when it doesn't exist |

`findings-scan-state` has no custom message, so it shows ACM's own. `compliancescans [<scan>] found but not as specified` is a bad scan; its diff shows `phase: Healthy`, a phase no scan reaches, asserted only to flag it. `compliancecheckresults not found` means no results yet.

What counts as a finding:

- `FAIL`, unless the rule is waived.
- `ERROR`, even when waived: the check didn't run, so there is nothing to waive.
- `INCONSISTENT`, which means the nodes of one role disagreed. It counts only when a node failed (unless waived) or errored, because most are `NOT-APPLICABLE` on some nodes.
- Never `MANUAL`: the operator can't check it. Review those yourself: `oc get ccr -n openshift-compliance -l compliance.openshift.io/check-status=MANUAL`.

A findings message is the count, then the result names: `2 findings: ocp4-stig-api-server-audit-log-maxsize, rhcos4-stig-worker-sysctl-kernel-dmesg-restrict`. With none, `No findings`.

!!! warning "Names are truncated"
    ACM 2.15+ caps a message at 4096 bytes, ending in `...[truncated]`. Names sort, so `rhcos4-*` is cut first. The count stays right. For the full list, use View details > Related resources on the template.

For the raw results, which also include waived rules and harmless `INCONSISTENT` ones:

```sh
oc get ccr -n openshift-compliance -l 'compliance.openshift.io/check-status in (FAIL,ERROR,INCONSISTENT)'
```

### Timing

A rescan keeps the last results, so findings stay visible while it runs. After a failed scan they are stale until a good one.

The findings templates re-evaluate as soon as results change. `findings-scan-state` can't, because a scan getting old fires no event: it polls hourly while Compliant and every 10 minutes while not, so a failed or stale scan takes up to an hour to show.

To scan now instead of waiting for the schedule, use Red Hat's rescan annotation:

```sh
oc -n openshift-compliance annotate compliancescans/<scan> compliance.openshift.io/rescan=
```

## Benchmarks

`config.profiles` takes any mix. They share one binding, schedule and suite, and result names carry the scan, so they don't collide.

```yaml
stack:
  compliance:
    config:
      profiles: [ocp4-cis, ocp4-cis-node, ocp4-stig, ocp4-stig-node, rhcos4-stig]
```

`profiles` is a list, so a layer's list replaces the default whole ([Merge rules](../concepts/values.md#merge-rules)): `[ocp4-cis]` alone drops STIG. A misspelt profile makes `findings-scan-state` NonCompliant.

!!! warning "Replacing a profile"
    Dropping a profile deletes its scan, results, remediations and raw-results PVC. Applied MachineConfigs, KubeletConfig settings and created objects stay behind. Set its remediations to `false` and roll out before you drop it; for KubeletConfig see [Reverting](#reverting).

Leave `config.binding` alone when you change profiles. A new name writes a second ScanSettingBinding, the old one is never deleted and keeps scanning, and the findings read only the new one. If you rename it, delete the old one: `oc delete ssb <old> -n openshift-compliance`.

## Pinning

Keep the floating profile names (`ocp4-stig`, not `ocp4-stig-v2r3`). Red Hat [supports only the latest](https://docs.redhat.com/en/documentation/openshift_container_platform/latest/html/security_and_compliance/compliance-operator#compliance-supported-profiles_compliance-operator-supported-profiles) CIS and STIG, a versioned name doesn't freeze its rules, and operator upgrades remove old versions.

The operator version freezes the content. Pin its CSV in each cluster's own layer, so a content bump, which can change results and remediations, reaches one cluster at a time ([Operator versions](../guides/operators.md#pinning)):

```yaml
# values/clusters/prod-east-1.yaml
stack:
  compliance:
    operatorPolicies:
      compliance:
        versions:
          - <installed CSV> # (1)!
        subscription:
          startingCSV: <installed CSV>
```

1.  `oc get csv -n openshift-compliance`.

A bump can change results:

- A rule dropped from the content loses its result and remediation, but an applied MachineConfig stays. The operator names them `75-<remediation>`; find leftovers with `oc get mc | grep ^75-`.
- A remediation whose fix changed is marked `Outdated` and keeps its old payload. It shows in `remediate-remediation-state`. To move it to the new payload, [remove the old one](https://docs.redhat.com/en/documentation/openshift_container_platform/latest/html/security_and_compliance/compliance-operator#compliance-updating_compliance-remediation). It may roll nodes:

    ```sh
    oc -n openshift-compliance patch complianceremediations <name> --type json -p '[{"op":"remove","path":"/spec/outdated"}]'
    ```

Don't use Red Hat's `compliance.openshift.io/apply-remediations` annotation: it applies every remediation in the suite, bypassing `config.remediations`.

## Waivers

A waiver accepts a failed rule. The rule keeps being scanned, so the raw `FAIL` stays on the cluster and in the raw results (ARF, the XML reports in the raw-results PVC) as evidence; the waiver only stops it counting as a finding. `git log` records who approved it.

```yaml
stack:
  compliance:
    config:
      waivers:
        api-server-audit-log-maxsize: "Logs ship off-cluster. SEC-1234" # (1)!
```

1.  The rule name, not the result name Governance shows: `oc get ccr -n openshift-compliance <result> -o jsonpath='{.metadata.annotations.compliance\.openshift\.io/rule}'`.

A waiver covers every result of that rule on the cluster, in every role and profile, and moves them from `findings-high`/`-medium`/`-low` to `findings-waived`. It takes effect at the next evaluation, with no rescan.

| You set | Result |
|---|---|
| A result name (starts `ocp4-` or `rhcos4-`) | Render fails: `stack.compliance.config.waivers.<name> is a result name: use its compliance.openshift.io/rule annotation` |
| An empty reason | Render fails: `stack.compliance.config.waivers.<rule> needs a reason` |
| A misspelt rule name | Nothing. It waives nothing; check that `findings-waived` lists it |
| `null` in a later layer (listed after it, or the cluster's own file) | Removes the waiver for the clusters reading that layer |

To waive on some clusters only, put the waiver in their layer. To lift an environment's waiver on one cluster, set the rule to `null` in that cluster's file.

!!! note "Limits"
    - The reason stays in Git. Governance doesn't show it.
    - `oc get compliancesuite -n openshift-compliance` still says `NON-COMPLIANT`, since the operator doesn't know about waivers.
    - Waivers don't expire. A waived rule that passed and fails again raises nothing; its entry in the `findings-waived` message just changes, e.g. `ocp4-stig-api-server-audit-log-maxsize=FAIL`.

To stop scanning a rule, Red Hat documents [TailoredProfile](https://docs.redhat.com/en/documentation/openshift_container_platform/latest/html/security_and_compliance/compliance-operator#compliance-tailored-profiles_compliance-tailor) `disableRules`. It removes the evidence, and it isn't part of this element.

## Remediations

For each failed rule that has an automated fix, the operator creates a ComplianceRemediation; list them with `oc get complianceremediations -n openshift-compliance`. Nothing is applied until you turn on policy `remediate` and declare each one:

```yaml
stack:
  compliance:
    policies:
      remediate:
        enabled: true
    config:
      remediations:
        ocp4-stig-api-server-encryption-provider-cipher: true # (1)!
        rhcos4-stig-worker-sysctl-kernel-dmesg-restrict: false # (2)!
```

1.  Applies it. When one fix needs several objects the operator creates `<name>`, `<name>-1`, `<name>-2`. List the base name: the operator sets the rest of the family to match. Two of a family with different values fail the render.
2.  Reverts it.

`true` sets the remediation's `spec.apply`, and the operator applies the payload: a MachineConfig (the Machine Config Operator then reboots that pool's nodes), a KubeletConfig, or an object it creates or patches. The result turns `PASS` only after the next scan.

- Both remediate templates do nothing while a scan runs, because the operator rewrites remediations mid-scan: `remediate-remediation-state` reads Compliant until every scan is `DONE`.
- A name that doesn't exist is never created. `remediate-remediation-state` reports a `true` one as `missing`, unless its result passes, since a passing rule has no remediation.
- A `false` name that doesn't exist isn't reported at all, so check the spelling.
- A value other than `true` or `false` fails the render.

Turning `remediate` off stops managing `spec.apply`. What is applied stays applied.

### Reverting

Removing an entry doesn't revert anything: it stays applied and shows in `findings-drift`. Set it to `false`, and remove the entry once its state is `NotApplied`.

| Remediation | `false` does |
|---|---|
| MachineConfig | Deletes it and reboots the pool |
| Object the operator created | Deletes it |
| KubeletConfig | Nothing. Status says `NotApplied`, the setting stays, and `remediate-remediation-state` stays NonCompliant until you follow [Red Hat's manual removal](https://docs.redhat.com/en/documentation/openshift_container_platform/latest/html/security_and_compliance/compliance-operator#compliance-removing-kubeletconfig_compliance-remediation) and drop the entry. Affected nodes reboot. |
| Object the operator patched | Nothing, and status says `NotApplied`. Nothing here detects it. |

Deleting a ComplianceRemediation doesn't revert it either. After any revert, [rescan](#timing): results don't change until the next scan.

## Turning it off

`enabled: false` deletes the Policies but nothing they created: no template sets `pruneObjectBehavior`, the ACM setting that deletes a template's objects with its Policy, and the operator stays installed ([Removal and pruning](../concepts/removal.md#what-happens-on-the-managed-cluster)). The binding keeps scanning daily and applied remediations stay. To stop scanning and keep the results:

```sh
oc patch scansetting default -n openshift-compliance --type merge -p '{"suspend":true}'
```

Deleting the binding (`oc delete ssb stig -n openshift-compliance`, or your `config.binding`) also deletes the results and raw-results PVCs.
