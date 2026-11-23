#!/usr/bin/env bash
# shellcheck disable=SC2016 # the expected texts hold literal Markdown code spans
# Tests stable-gate.sh: each case edits a throwaway clone of HEAD, commits there, and checks what the
# gate reports against HEAD. Run from the repo root with the validator built; upstream.yml runs it.
set -euo pipefail

gate=$PWD/.github/scripts/stable-gate.sh
export VALIDATOR=$PWD/tools/validator/bin/policystack-validator
tmp=$(mktemp -d)
tree=$tmp/tree
trap 'rm -rf "$tmp"' EXIT
orig=$(git rev-parse HEAD)
git clone -q --shared --no-checkout . "$tree"
fails=0

# check <name> <found> <expected body text, or ''> <edit>
check() {
  local name=$1 want=$2 text=$3 edit=$4 out
  git -C "$tree" checkout -qf --detach "$orig" && git -C "$tree" clean -qfdx -e charts
  (cd "$tree" && eval "$edit")
  git -C "$tree" add -A
  git -C "$tree" -c user.name=test -c user.email=test@example.com -c commit.gpgsign=false commit -qm "$name" --allow-empty
  export RUNNER_TEMP=$tmp/$name GITHUB_OUTPUT=$tmp/$name.out GITHUB_STEP_SUMMARY=/dev/null
  mkdir -p "$RUNNER_TEMP"
  if ! (cd "$tree" && "$gate" "$orig" HEAD) > "$tmp/$name.log" 2>&1; then
    echo "FAIL $name: the gate exited non-zero"; tail -20 "$tmp/$name.log"; fails=$((fails + 1)); return
  fi
  out=$(cat "$RUNNER_TEMP/stable-gate/body.md")
  if ! grep -qx "found=$want" "$GITHUB_OUTPUT" || { [ -n "$text" ] && ! grep -qF -- "$text" <<< "$out"; }; then
    echo "FAIL $name: want found=$want and '$text', got:"; echo "$out"; fails=$((fails + 1)); return
  fi
  echo "ok   $name"
}

y() { yq -i "$1" "$2"; }

check unchanged false 'No stable element changes.' 'true'
check preview-break false '' 'y ".stack.loki.policies = {}" stack/loki/values.yaml'
check config-removed true '`config.namespace` removed' 'y "del(.stack.gitops.config.namespace)" stack/gitops/values.yaml'
check config-type true '`config.serverAutoscale.maxReplicas` changes type: number → string' \
  'y ".stack.gitops.config.serverAutoscale.maxReplicas = \"3\"" stack/gitops/values.yaml'
check policy-removed true 'policy `link` removed' 'y "del(.stack.gitops.policies.link)" stack/gitops/values.yaml && rm -r stack/gitops/converters/link'
check converter-renamed true 'converter `converters/link/argocd-console.yaml` removed or renamed' \
  'git mv stack/gitops/converters/link/argocd-console.yaml stack/gitops/converters/link/console.yaml'
check default-flipped true '`default.remediationAction` default: "enforce" → "inform"' \
  'y ".stack.gitops.default.remediationAction = \"inform\"" stack/gitops/values.yaml'
check template-compliance true '`policies.instance.templates.argocd.complianceType` default: unset → "mustonlyhave"' \
  'y ".stack.gitops.policies.instance.templates.argocd.complianceType = \"mustonlyhave\"" stack/gitops/values.yaml'
check floor-major true 'library floor moves to major 5' \
  'y "(.dependencies[] | select(.name == \"policy-library\")).version = \"^5.0.0\"" stack/gitops/Chart.yaml'
check demoted true 'no longer `stable`' 'y ".annotations[\"policystk.io/stability\"] = \"preview\"" stack/upgrade/Chart.yaml'
check element-removed true 'element removed' 'git rm -rq stack/upgrade'
check promoted true '| `odf` | promoted to `stable` |' 'y ".annotations[\"policystk.io/stability\"] = \"stable\"" stack/odf/Chart.yaml'
check output-changed true '#### Output changed for unchanged values' \
  'sed -i "0,/^metadata:/s//metadata:\n  labels: {test: x}/" stack/gitops/converters/link/argocd-console.yaml'
check render-fails true 'render fails for unchanged values' 'echo "{{ fail \"x\" }}" >> stack/gitops/converters/link/argocd-console.yaml'
check new-policy-off false 'No stable element changes.' \
  'y ".stack.gitops.policies.extra = {\"enabled\": false, \"description\": \"x\", \"templates\": {\"cm\": {\"description\": \"x\"}}}" stack/gitops/values.yaml &&
   mkdir -p stack/gitops/converters/extra &&
   printf "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: extra\n  namespace: default\n" > stack/gitops/converters/extra/cm.yaml'
# On by default, a new entry is an output change for unchanged values.
check new-policy-on true 'added: `Policy/extra`' \
  'y ".stack.acm.policies.extra = {\"description\": \"x\", \"templates\": {\"cm\": {\"description\": \"x\"}}}" stack/acm/values.yaml &&
   mkdir -p stack/acm/converters/extra &&
   printf "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: extra\n  namespace: default\n" > stack/acm/converters/extra/cm.yaml'
# PR text reaches the comment without the characters that break a table or code span.
check sanitized true '`default.remediationAction` default: "enforce" → "enforce"' \
  'y ".stack.gitops.default.remediationAction = \"en|f\`o<rce\"" stack/gitops/values.yaml'
# A values.yaml that doesn't parse reads as every key removed, never as no change.
check values-unreadable true '`config.namespace` removed' 'printf "bad: [\n" >> stack/gitops/values.yaml'
# A Policy a stable element depends on, renamed in another element, preview or not.
check dependency-renamed true 'dependency no longer resolves: policies.observability: dependency "ready" is not a policy of element "odf"' \
  'y ".stack.odf.policies.readyz = .stack.odf.policies.ready | del(.stack.odf.policies.ready)" stack/odf/values.yaml &&
   git mv stack/odf/converters/ready stack/odf/converters/readyz'
# An enforcement default that comes from policy-library, not the element: Breaking from the render.
check library-default true '`complianceType` "musthave" → "mustonlyhave" in `Policy/' \
  'sed -i "s/complianceType | default \"musthave\"/complianceType | default \"mustonlyhave\"/" lib/policy-library/templates/_policies.tpl'
# A template turned off by default: its ConfigurationPolicy leaves an existing Policy.
check template-off true 'policy template `instance-ca-bundle` removed' \
  'y ".stack.gitops.policies.instance.templates.ca-bundle.enabled = false" stack/gitops/values.yaml'
# The PR's own test values are ignored: the base's render both sides.
check masked-inputs true '`config.namespace` removed' \
  'y "del(.stack.gitops.config.namespace)" stack/gitops/values.yaml && y ".stack.gitops.config.namespace = \"openshift-gitops\"" tools/validator/testdata/baseline.yaml'

[ "$fails" = 0 ] || { echo "$fails case(s) failed"; exit 1; }
