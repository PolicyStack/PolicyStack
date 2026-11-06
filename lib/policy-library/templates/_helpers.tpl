{{/*
Helper function to convert chart name to camelCase
*/}}
{{- define "policy-library.componentName" -}}
{{- .Chart.Name | replace "-" " " | replace "_" " " | title | nospace | untitle -}}
{{- end -}}

{{/*
Whether an entry (policy, template, operator, policySet) renders. Every entry is on unless it sets
enabled: false. A null counts as absent, since under alias: stack Helm 3.17+ keeps a layer's null
instead of deleting the key.

Returns the string "true" when enabled, otherwise the empty string.

Args: the entry
*/}}
{{- define "policy-library.enabled" -}}
{{- if ne (toString (. | default dict).enabled) "false" }}true{{ end -}}
{{- end -}}

{{/*
The element's Policies and their templates, as JSON: [{name, templates: [{name, files}], operators}].

converters/<policy>/<template>.yaml is template <template> of Policy <policy>; a directory or file
whose name starts with _ is a partial and never discovered. policies.<p>.templates.<t> sets options on
a template, and its file (a path under converters/ without .yaml, or a list of them) replaces the
discovered one. Operators always render into the install Policy. Templates render in name order;
policies.<p>.orderManifests lists the ones that come first, in that order, and a non-empty list makes
every template and operator of the Policy wait for the one rendered before it. Operators come last, so
an ordered install template that waits on one fails.

A policy or template no file backs fails, which catches a values layer key that names nothing.
Disabled entries are dropped.

Args: dict "root" <root context> "component" <stack component>
*/}}
{{- define "policy-library.model" -}}
{{- $root := .root -}}
{{- $declared := .component.policies | default dict -}}
{{- $found := dict -}}
{{- range $path, $_ := $root.Files.Glob "converters/*/*.yaml" -}}
{{- $p := base (dir $path) -}}
{{- $t := base $path | trimSuffix ".yaml" -}}
{{- if not (or (hasPrefix "_" $p) (hasPrefix "_" $t)) -}}
{{- $_ := set $found $p (set (get $found $p | default dict) $t (list (printf "%s/%s" $p $t))) -}}
{{- end -}}
{{- end -}}
{{- $allOperators := keys (.component.operatorPolicies | default dict) -}}
{{- $operators := list -}}
{{- range $name, $op := .component.operatorPolicies | default dict -}}
{{- if and (kindIs "map" $op) (include "policy-library.enabled" $op) }}{{ $operators = append $operators $name }}{{ end -}}
{{- end -}}
{{- $names := concat (keys $found) (keys $declared) -}}
{{- if $allOperators }}{{ $names = append $names "install" }}{{ end -}}
{{- $out := list -}}
{{- range $p := sortAlpha (uniq $names) -}}
{{- $pv := get $declared $p | default dict -}}
{{- $tdecl := $pv.templates | default dict -}}
{{- $tfound := get $found $p | default dict -}}
{{- $tnames := sortAlpha (uniq (concat (keys $tfound) (keys $tdecl))) -}}
{{- $isInstall := and (eq $p "install") (not (empty $allOperators)) -}}
{{- if not (or $tnames $isInstall) -}}
{{- fail (printf "policy-library: policies.%s has no templates - add converters/%s/<template>.yaml" $p $p) -}}
{{- end -}}
{{- $order := $pv.orderManifests | default list -}}
{{- $rest := list -}}
{{- range $i, $t := $order -}}
{{- if not (has $t $tnames) -}}
{{- fail (printf "policy-library: policies.%s.orderManifests names %q, which is not a template of the policy" $p $t) -}}
{{- end -}}
{{- if has $t (slice $order 0 $i) -}}
{{- fail (printf "policy-library: policies.%s.orderManifests lists %q twice" $p $t) -}}
{{- end -}}
{{- end -}}
{{- range $tnames }}{{ if not (has . $order) }}{{ $rest = append $rest . }}{{ end }}{{ end -}}
{{- $templates := list -}}
{{- range $t := concat $order $rest -}}
{{- $tv := get $tdecl $t | default dict -}}
{{- $files := get $tfound $t | default list -}}
{{- with $tv.file }}{{ $files = ternary (list .) . (kindIs "string" .) }}{{ end -}}
{{- if not $files -}}
{{- fail (printf "policy-library: policies.%s.templates.%s has no file - add converters/%s/%s.yaml or set file" $p $t $p $t) -}}
{{- end -}}
{{- if include "policy-library.enabled" $tv -}}
{{- if and $isInstall $order $tv.waitForOperator -}}
{{- fail (printf "policy-library: policies.install.orderManifests chains operators after templates, so template %q can't wait on an operator - move the ordered templates to another policy" $t) -}}
{{- end -}}
{{- $templates = append $templates (dict "name" $t "files" $files) -}}
{{- end -}}
{{- end -}}
{{- if include "policy-library.enabled" $pv -}}
{{- $out = append $out (dict "name" $p "templates" $templates "operators" (ternary $operators list $isInstall)) -}}
{{- end -}}
{{- end -}}
{{- toJson $out -}}
{{- end -}}

{{/*
The release name is "<chart>-<cluster>" (the ApplicationSet names each Application
"<element>-<cluster>", and Argo CD uses that as the Helm release name). Strip the chart-name prefix
to recover the cluster suffix, which is the half a sibling element shares with us. Derived from
Release.Name rather than a value such as .Values.selectedName so it also works under a plain
`helm template` and in CI, where no ApplicationSet has injected anything.

Returns the empty string when Release.Name does not start with "<chart>-".
*/}}
{{- define "policy-library.clusterSuffix" -}}
{{- if hasPrefix (printf "%s-" .Chart.Name) .Release.Name -}}
{{- trimPrefix (printf "%s-" .Chart.Name) .Release.Name -}}
{{- end -}}
{{- end -}}

{{/*
Name of a policy-template object (ConfigurationPolicy, OperatorPolicy):
"<policyRef>-<name>", plus "-<cluster>" when the component sets suffixTemplateNames.

ACM requires template names to be unique across every Policy placed on a cluster. Elements render
one release per cluster, so the bare name is unique. A chart that renders one release per cluster
but places every release on the same cluster (e.g. the hub) sets suffixTemplateNames instead.

Args: dict "root" <root context> "component" <stack component> "policyRef" <policy> "name" <entry>
*/}}
{{- define "policy-library.templateName" -}}
{{- $name := printf "%s-%s" .policyRef .name -}}
{{- if (.component).suffixTemplateNames -}}
  {{- $suffix := include "policy-library.clusterSuffix" .root -}}
  {{- if not $suffix -}}
    {{- fail (printf "policy-library: suffixTemplateNames requires Release.Name (%q) to start with the chart name %q" .root.Release.Name .root.Chart.Name) -}}
  {{- end -}}
  {{- $name = printf "%s-%s" $name $suffix -}}
{{- end -}}
{{- $name -}}
{{- end -}}

{{/*
Resolve a list of dependency entries into ACM PolicyDependency YAML.

Each entry accepts: name (required), kind, apiVersion, namespace, compliance, release, element,
policyRef, raw.
Names are resolved using the chart's own naming rules so values files can refer to policies and
sub-policies by the names they were declared with:
  - kind Policy         -> "<name>-<release>"      (release defaults to .Release.Name)
  - element: <chart>    -> "<name>-<element>-<cluster>", where <cluster> is this release's own
                           cluster suffix. Use this to depend on a Policy owned by a DIFFERENT
                           PolicyStack element on the SAME cluster: every element renders as its own
                           Helm release named "<chart>-<cluster>", so the target's release name
                           differs only in the element half. Unlike "release", this stays correct on
                           every cluster, so it can live in a shared values file.
  - template kinds      -> "<policyRef>-<name>"    (policyRef defaults to the owning policy;
                           "-<cluster>" is appended under suffixTemplateNames)
  - raw: true           -> name is used verbatim
Precedence for kind Policy: raw > release > element > .Release.Name.
Namespace is only emitted for kind Policy. Template kinds (ConfigurationPolicy, OperatorPolicy) live
in the per-cluster namespace on the managed cluster, so ACM resolves that itself and the field must be
left off.

Args: dict "deps" <list> "root" <root context> "component" <stack component>
           "policyRef" <owning policy name> "defaultKind" <kind>
*/}}
{{- define "policy-library.dependencyList" -}}
{{- $root := .root -}}
{{- $policyRef := .policyRef -}}
{{- $defaultKind := .defaultKind | default "Policy" -}}
{{- $out := list -}}
{{- $clusterSuffix := include "policy-library.clusterSuffix" $root -}}
{{- range .deps -}}
  {{- if not .name -}}
    {{- fail (printf "policy-library: dependency entry requires a 'name' (policy %q)" $policyRef) -}}
  {{- end -}}
  {{- $kind := .kind | default $defaultKind -}}
  {{- $apiVersion := .apiVersion -}}
  {{- if not $apiVersion -}}
    {{- if eq $kind "OperatorPolicy" -}}
      {{- $apiVersion = "policy.open-cluster-management.io/v1beta1" -}}
    {{- else if has $kind (list "Policy" "ConfigurationPolicy") -}}
      {{- $apiVersion = "policy.open-cluster-management.io/v1" -}}
    {{- else -}}
      {{- fail (printf "policy-library: unknown dependency kind %q on %q (policy %q) - set apiVersion explicitly" $kind .name $policyRef) -}}
    {{- end -}}
  {{- end -}}
  {{- $name := .name -}}
  {{- if not .raw -}}
    {{- if eq $kind "Policy" -}}
      {{- if .release -}}
        {{- $name = printf "%s-%s" .name .release -}}
      {{- else if .element -}}
        {{- if not $clusterSuffix -}}
          {{- fail (printf "policy-library: dependency element:%q on policy %q requires Release.Name (%q) to start with the chart name %q - install with the release name PolicyStack's ApplicationSet uses, or set 'release' explicitly" .element $policyRef $root.Release.Name $root.Chart.Name) -}}
        {{- end -}}
        {{- $name = printf "%s-%s-%s" .name .element $clusterSuffix -}}
      {{- else -}}
        {{- $name = printf "%s-%s" .name $root.Release.Name -}}
      {{- end -}}
    {{- else -}}
      {{- if .element -}}
        {{- fail (printf "policy-library: dependency element:%q on policy %q is only valid for kind Policy - %s objects are namespaced to the managed cluster and carry no release suffix, so depend on the owning Policy instead" .element $policyRef $kind) -}}
      {{- end -}}
      {{- $name = include "policy-library.templateName" (dict "root" $root "component" $.component "policyRef" (.policyRef | default $policyRef) "name" .name) -}}
    {{- end -}}
  {{- end -}}
  {{- $namespace := .namespace -}}
  {{- if and (not $namespace) (eq $kind "Policy") -}}
    {{- $namespace = $root.Values.policyNamespace -}}
  {{- end -}}
  {{- $entry := dict "apiVersion" $apiVersion "kind" $kind "name" $name "compliance" (.compliance | default "Compliant") -}}
  {{- if $namespace -}}
    {{- $_ := set $entry "namespace" $namespace -}}
  {{- end -}}
  {{- $out = append $out $entry -}}
{{- end -}}
{{- if $out -}}
{{- toYaml $out -}}
{{- end -}}
{{- end -}}

{{/*
Build the complete extraDependencies YAML for a single policy-template entry.
Combines, in order: the orderManifests dependency (if any), waitForOperator shorthand, and the
template's own extraDependencies. ACM ANDs all entries, so concatenation is well defined.
Returns an empty string when there is nothing to emit.

waitForOperator names a key in operatorPolicies, which always render into the install Policy; a
disabled install Policy fails.

Args: dict "root" <root context> "component" <stack component> "policyRef" <owning policy name>
           "subPolicy" <template or operator entry, or an empty dict>
           "prev" <dict with name+kind to chain from, or an empty dict>
*/}}
{{- define "policy-library.subPolicyDependencies" -}}
{{- $root := .root -}}
{{- $component := .component -}}
{{- $policyRef := .policyRef -}}
{{- $sub := .subPolicy | default dict -}}
{{- $deps := list -}}
{{- if .prev -}}
  {{- $deps = append $deps (dict "name" .prev.name "kind" .prev.kind "raw" true) -}}
{{- end -}}
{{- $waitFor := $sub.waitForOperator | default list -}}
{{- if kindIs "string" $waitFor -}}
  {{- $waitFor = list $waitFor -}}
{{- end -}}
{{- range $operator := $waitFor -}}
  {{- $op := index ($component.operatorPolicies | default dict) $operator -}}
  {{- if not (and (kindIs "map" $op) (include "policy-library.enabled" $op)) -}}
    {{- fail (printf "policy-library: waitForOperator %q on policy %q matches no enabled entry in operatorPolicies" $operator $policyRef) -}}
  {{- end -}}
  {{- if not (include "policy-library.enabled" (get ($component.policies | default dict) "install")) -}}
    {{- fail (printf "policy-library: waitForOperator %q on policy %q needs policies.install, which is disabled" $operator $policyRef) -}}
  {{- end -}}
  {{- if eq $op.complianceType "mustnothave" -}}
    {{- fail (printf "policy-library: waitForOperator %q on policy %q names an operator with complianceType mustnothave, which is being uninstalled" $operator $policyRef) -}}
  {{- end -}}
  {{- $deps = append $deps (dict "name" (printf "%s-status" (include "policy-library.templateName" (dict "root" $root "component" $component "policyRef" "install" "name" $operator))) "kind" "ConfigurationPolicy" "raw" true) -}}
{{- end -}}
{{- $deps = concat $deps ($sub.extraDependencies | default list) -}}
{{- if $deps -}}
{{- include "policy-library.dependencyList" (dict "deps" $deps "root" $root "component" $component "policyRef" $policyRef "defaultKind" "ConfigurationPolicy") -}}
{{- end -}}
{{- end -}}

{{/*
Emit the leading keys of a policy-templates[] list item, up to and including "objectDefinition:".
Collapses to the plain "- objectDefinition:" when the entry has neither dependencies nor
ignorePending.

Args: dict "depsYaml" <output of policy-library.subPolicyDependencies> "ignorePending" <bool>
*/}}
{{- define "policy-library.policyTemplateHeader" -}}
{{- if .depsYaml -}}
- extraDependencies:
{{ .depsYaml | indent 4 }}
{{- if .ignorePending }}
  ignorePending: true
{{- end }}
  objectDefinition:
{{- else if .ignorePending -}}
- ignorePending: true
  objectDefinition:
{{- else -}}
- objectDefinition:
{{- end -}}
{{- end -}}

{{/*
Wrapper template that can be called from consuming charts
*/}}
{{- define "policy-library.render" -}}
{{- include "policy-library.renderPolicies" . -}}
{{- include "policy-library.renderPlacements" . -}}
{{- include "policy-library.renderPolicySets" . -}}
{{- end -}}
