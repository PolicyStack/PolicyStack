# Operator versions

Each `operatorPolicies` entry renders OperatorPolicy `install-<key>`, with `install-<key>-ns` and `install-<key>-status`, in the element's `install` Policy ([Operators](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md#operators)). Elements ship unpinned.

| Goal | Set on the key | Result when enforced |
|---|---|---|
| Follow the channel | `versions` empty (default). To unpin, `versions: []` in a higher layer, or delete the pin | OLM installs every upgrade the channel offers. |
| Pin | `versions` | Approves an InstallPlan only for a CSV in `versions` or `subscription.startingCSV`. |
| Install once | `upgradeApproval: None` | Approves the first install only, never an upgrade. |
| Stop managing | `enabled: false` (never `null`) | Stays installed. |
| Uninstall | `complianceType: mustnothave` | Deletes Subscription, CSV, and an unused OperatorGroup. Keeps CRDs and namespace ([Removing an operator](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md#removing-an-operator)). |

- ACM rejects `Manual` and `subscription.installPlanApproval`.
- Inform approves nothing; it flags an installed CSV missing from a non-empty `versions`.
- `versions` holds exact CSV names, no wildcards.
- The hub's ACM and GitOps follow the same rules once their elements are on ([Hand the operators to PolicyStack](../start/install.md#hand-the-operators-to-policystack)).

## Pinning

`operatorPolicies` is a map, so a layer sets only what it changes ([Merge rules](../concepts/values.md#merge-rules)):

```yaml
# values/environments/prod.yaml
stack:
  metallb:
    operatorPolicies:
      metallb: # (1)!
        versions:
          - <installed CSV> # (2)!
        subscription:
          startingCSV: <installed CSV> # (3)!
```

1.  Must be a key the element defines. A new key fails POLICY032 ([Validator rules](../reference/rules.md)). The render also fails unless the layer sets `subscription.name`.
2.  `oc get csv -n metallb-system` on the managed cluster.
3.  Without it, a fresh cluster resolves the channel head, which the pin blocks. Omit it when the pin is the head.

## Bumping

Watch the OperatorPolicy status for `an InstallPlan to update to [<csv>]`. OLM picks that CSV from the upgrade graph; the policy only approves or holds it.

1. Add the new CSV next to the old one. Promote cluster by cluster ([Promoting and rolling back](rollout.md#promoting-and-rolling-back)).
2. Once all clusters run it, drop the old CSV, move `startingCSV`, and promote again.

Swapping old for new in one change makes every cluster NonCompliant until it upgrades.

## Uninstalling

Stop managing and Uninstall both fail the render unless templates naming the key in `waitForOperator` are turned off in the same layer.

```yaml
# values/clusters/prod-east-1.yaml: uninstall metallb
stack:
  metallb:
    policies:
      config:
        templates:
          metallb-instance:
            enabled: false # (1)!
    operatorPolicies:
      metallb:
        complianceType: mustnothave # (2)!
```

1.  The instance waits on `metallb`, so it goes off in the same layer.
2.  `removalBehavior` changes what is deleted.
