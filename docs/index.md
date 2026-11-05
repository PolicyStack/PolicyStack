# PolicyStack

PolicyStack keeps OpenShift cluster configuration in Git as Red Hat Advanced Cluster Management (ACM) policies. Each directory under `stack/` is an element: a Helm chart that renders policies through the [policy-library](https://github.com/PolicyStack/PolicyStack-chart/blob/main/charts/policy-library/README.md) chart. Its `converters/<policy>/<template>.yaml` files and `operatorPolicies` define the policies; values configure them. On the hub, an Argo CD ApplicationSet creates one Application per element for each cluster that has a fleet file. Argo CD syncs the policies to the hub only, and ACM delivers each one to the single cluster it targets.

```mermaid
flowchart LR
  subgraph git["Git"]
    fleet["fleet/#lt;cluster#gt;.yaml<br>default branch"]
    stack["stack/#lt;element#gt;/"]
    vals["values.yaml<br>values/**"]
  end
  subgraph hub["ACM hub"]
    appset["ApplicationSet<br>policystack"]
    app["Application<br>#lt;element#gt;-#lt;cluster#gt;"]
    pol["Policies in pstack"]
  end
  spoke["Managed cluster<br>governance add-on"]
  fleet -->|"revision, valueFiles"| appset
  stack --> appset
  appset -->|"one per element × cluster"| app
  vals --> app
  app -->|"Helm render, sync"| pol
  pol -->|"Placement: one cluster"| spoke
  spoke -.->|"compliance"| pol
```

## Documentation

| Section | Read it when | Start at |
|---|---|---|
| Get started | You are setting up a hub | [Installation](start/install.md) |
| Concepts | You want to know how a commit reaches a cluster | [Architecture](concepts/architecture.md) |
| Guides | You run a hub: rollouts, upgrades, cluster lifecycle | [Rolling out changes](guides/rollout.md) |
| Elements | You configure an element | Its [`stack/<element>/README.md`](https://github.com/PolicyStack/PolicyStack/tree/main/stack) on GitHub; [ACS](elements/acs.md) and [Compliance](elements/compliance.md) have guides |
| Develop | You write or change an element | [Writing an element](develop/elements.md) |
| Reference | You look up a key, rule or workflow | [Fleet files](reference/fleet.md) |
| Contributing | You change the framework itself | [Contributing](contributing.md) |
