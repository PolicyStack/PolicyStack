#!/usr/bin/env bash
# Compares every stable element between two commits and writes a comment body listing what a
# maintainer must sign off on (docs/reference/ci.md#stable). Run from a trusted checkout with the
# validator built: the commits are only read and rendered, never executed.
#
# Usage: stable-gate.sh <base> <head>
# Env: RUNNER_TEMP, GITHUB_OUTPUT, GITHUB_STEP_SUMMARY; RUN_URL and VALIDATOR optional.
# Writes $RUNNER_TEMP/stable-gate/{body.md,stable-gate.patch} and sets found=true|false and body.
set -euo pipefail

base=$(git rev-parse "$1^{commit}") head=$(git rev-parse "$2^{commit}")
bin=${VALIDATOR:-$PWD/tools/validator/bin/policystack-validator}
work=$RUNNER_TEMP/stable-gate
rm -rf "$work" && git worktree prune && mkdir -p "$work/render" "$work/details" "$work/log"
git worktree add -q --detach "$work/base" "$base"
git worktree add -q --detach "$work/head" "$head"
# Both sides render with the base's test values, so a PR can't hide a change by editing them.
data=$work/base/tools/validator/testdata

breaks=() outputs=() promoted=() # Markdown table rows
# md: PR text made safe for a table cell or a code span
md() { tr -d '`\n' | sed -e 's/|/\\|/g' -e 's/</\&lt;/g'; }
# shellcheck disable=SC2016 # literal Markdown code span
row() { printf '| `%s` | %s |' "$1" "$2"; }

# stable <tree>: the elements whose Chart.yaml says stable
stable() {
  local c
  for c in "$1"/stack/*/Chart.yaml; do
    if [ "$(yq '.annotations["policystk.io/stability"]' "$c")" = stable ]; then basename "$(dirname "$c")"; fi
  done
}
# floor <Chart.yaml>: the policy-library major the element needs
floor() { yq '.dependencies[] | select(.name == "policy-library") | .version' "$1" | grep -oE '[0-9]+' | head -1 || true; }
# converters <element dir>: converters/<policy>/<template>.yaml, partials left out
converters() { (cd "$1" && find converters -name '*.yaml' ! -name '_*' 2>/dev/null | sort); }
# values <file>: the element's block under stack, as JSON; {} when the file is missing or doesn't
# parse, so every key reads as removed
values() { yq -o=json '.stack | to_entries | .[0].value // {}' "$1" 2>/dev/null || echo '{}'; }

# changes <base values> <head values>: one Markdown line per key change the contract calls breaking
changes() {
  jq -rn --argjson b "$1" --argjson h "$2" '
    def safe: tostring | gsub("[`|<\n]"; "");
    def key: map(safe) | join(".");
    def val: if .ok then .v | tojson | safe else "unset" end;
    # leaves: {p, t} for every value that is not a non-empty map; a list is one leaf
    def leaves: def l($p): if type == "object" and length > 0
      then to_entries[] as $e | $e.value | l($p + [$e.key]) else {p: $p, t: type} end; l([]);
    # at($p): {ok, v}; ok is false when a key on the way is missing
    def at($p): reduce $p[] as $k ({v: ., ok: true};
      if .ok and (.v | type) == "object" and (.v | has($k)) then {v: .v[$k], ok: true} else {ok: false} end);
    def enforcement: [paths(type != "object") | select(.[0] | IN("enabled", "default", "policies", "operatorPolicies"))
      | select(.[-1] | IN("enabled", "remediationAction", "complianceType", "pruneObjectBehavior"))];
    # Each check in parentheses: a comma binds tighter than a pipe.
    (($b.config // {} | leaves) as $l | ($h.config // {} | at($l.p)) as $x
      | if ($x.ok | not) then "`config.\($l.p | key)` removed"
        elif ($x.v | type) != $l.t and $l.t != "null" and ($x.v | type) != "null"
        then "`config.\($l.p | key)` changes type: \($l.t) → \($x.v | type)"
        else empty end),
    (($b.policies // {} | keys[]) as $p
      | if ($h.policies // {} | has($p) | not) then "policy `\($p | safe)` removed"
        else ($b.policies[$p].templates // {} | keys[]) as $t
          | select($h.policies[$p].templates // {} | has($t) | not) | "template `\([$p, $t] | key)` removed" end),
    (($b.operatorPolicies // {} | keys[]) | select(. as $o | $h.operatorPolicies // {} | has($o) | not)
      | "operator `\(safe)` removed"),
    ((($b | enforcement) + ($h | enforcement) | unique[]) as $p
      | ($b | at($p)) as $x | ($h | at($p)) as $y
      | select($x != $y)
      # Only where the entry is on both sides: a new entry is an addition, a removed one is above.
      | select(($b | at($p[:-1]).ok) and ($h | at($p[:-1]).ok))
      | "`\($p | key)` default: \($x | val) → \($y | val)")'
}

# Key and file checks on every element stable at the base.
mapfile -t was < <(stable "$work/base")
mapfile -t now < <(stable "$work/head")
render=()
for el in "${was[@]}"; do
  b=$work/base/stack/$el h=$work/head/stack/$el
  if [ ! -d "$h" ]; then breaks+=("$(row "$el" "element removed")"); continue; fi
  render+=("$el")
  [[ " ${now[*]} " == *" $el "* ]] || breaks+=("$(row "$el" "no longer \`stable\`")")
  [ "$(floor "$b/Chart.yaml")" = "$(floor "$h/Chart.yaml")" ] ||
    breaks+=("$(row "$el" "library floor moves to major $(floor "$h/Chart.yaml" | md)")")
  while read -r f; do
    [ -z "$f" ] || breaks+=("$(row "$el" "converter \`$(md <<< "$f")\` removed or renamed")")
  done < <(comm -23 <(converters "$b") <(converters "$h"))
  keyed=$(changes "$(values "$b/values.yaml")" "$(values "$h/values.yaml")") # a failure stops the gate
  while read -r line; do
    [ -z "$line" ] || breaks+=("$(row "$el" "$line")")
  done <<< "$keyed"
done
for el in "${now[@]}"; do
  [[ " ${was[*]} " == *" $el "* ]] || promoted+=("$(row "$el" "promoted to \`stable\`")")
done

# The renders validate makes, the element turned on in each: the fixture fleet with its values,
# which is what an adopter using the element gets, then everything on, then each profile.
passes=(defaults all-on)
for p in "$data"/profiles/*.yaml; do [ ! -f "$p" ] || passes+=("$(basename "$p" .yaml)"); done
for el in "${render[@]}"; do
  printf 'stack:\n  %s:\n    enabled: true\n' "$(yq '.stack | keys | .[0]' "$work/base/stack/$el/values.yaml")" > "$work/on-$el.yaml"
done
# flags <pass> <element>
flags() {
  case $1 in
    defaults) echo "--extra-values $data/baseline.yaml,$work/on-$2.yaml" ;;
    all-on) echo "--all-on --extra-values $data/baseline.yaml,$data/inputs.yaml" ;;
    *) echo "--extra-values $data/baseline.yaml,$work/on-$2.yaml,$data/profiles/$1.yaml" ;;
  esac
}
for side in base head; do
  for el in "${render[@]}"; do
    for pass in "${passes[@]}"; do
      echo "::group::Render $el ($pass) at $side"
      rc=0 log=$work/log/$side-$el-$pass
      # POLICY011 also resolves the dependencies on other elements' Policies the contract holds.
      # shellcheck disable=SC2046 # flags is a list of words
      "$bin" --repo-root "$work/$side" --values-dir "$work/base/values" --fixtures-dir "$data/clusters" \
        --element "$el" $(flags "$pass" "$el") --only RENDER000,POLICY011 --render-dir "$work/render/$side/$pass" > "$log" || rc=$?
      cat "$log"
      echo "::endgroup::"
      # Exit 1 is a failed render, written into the render as # RENDER000. Above 1 the tree did not load.
      if [ "$rc" -gt 1 ]; then
        [ "$side" = head ] || { echo "::error::The validator cannot load the base (exit $rc)."; exit 1; }
        breaks+=("$(row "$el" "the validator cannot load this PR's tree ($pass, exit $rc)")")
      fi
    done
  done
done

# A dependency that resolves at the base and not in this PR: a Policy it names was renamed or removed.
# dependencies <side> <element>: the element's POLICY011 messages across its renders
dependencies() { cat "$work/log/$1-$2-"* | sed -n "s/^error POLICY011 \[$2\] //p" | sort -u; }
for el in "${render[@]}"; do
  while read -r m; do
    [ -z "$m" ] || breaks+=("$(row "$el" "dependency no longer resolves: $(md <<< "$m")")")
  done < <(comm -13 <(dependencies base "$el") <(dependencies head "$el"))
done

cd "$work/render"
# objects <file> <release>: {n, k, v} per object, k is Kind/<entry> with the -<release> suffix cut,
# or Kind for the element's own Placement and PlacementBinding
objects() {
  [ ! -f "$1" ] || yq -o=json -I=0 'select(.kind != null) | {"n": .metadata.name, "k": .kind, "v": .}' "$1" |
    jq -c --arg s "$2" '.k += (if .n == $s then "" else "/" + (.n | rtrimstr("-" + $s)) end)'
}
# compare <base> <head> <release> <defaults>: the entry keys added, removed and changed, and the
# enforcement each Policy on both sides renders differently, wherever its default comes from: values,
# converter or library. Outside the defaults pass, which alone shows what renders for unchanged
# values, new entries are additions that ship off: they and the PlacementBinding subjects and
# PolicySet entries naming them are left out.
compare() {
  jq -cn --slurpfile b <(objects "$1" "$3") --slurpfile a <(objects "$2" "$3") --argjson defaults "$4" '
    ($b | map({(.k): .v}) | add // {}) as $bm
    | ([$a[] | select($bm[.k] == null) | .n]) as $new
    | def drop: if .kind == "PlacementBinding" then .subjects |= map(select(.name as $n | $new | index($n) | not))
        elif .kind == "PolicySet" then .spec.policies |= map(select(. as $n | $new | index($n) | not)) else . end;
    ($a | map({(.k): (if $defaults then .v else .v | drop end)}) | add // {}) as $am
    | def s: tostring | gsub("[`|<\n]"; "");
      def val: if . == null then "unset" else tojson | s end;
      def enforcement: {remediationAction: .spec.remediationAction, disabled: .spec.disabled,
        templates: ([.spec["policy-templates"][]?.objectDefinition
          | select(.kind | IN("ConfigurationPolicy", "OperatorPolicy", "CertificatePolicy"))
          | {(.metadata.name | s): {remediationAction: .spec.remediationAction,
              pruneObjectBehavior: .spec.pruneObjectBehavior, complianceType: .spec.complianceType,
              objects: ([.spec["object-templates"][]?
                | {("\(.objectDefinition.kind)/\(.objectDefinition.metadata.name)" | s): .complianceType}] | add // {})}}]
          | add // {})};
      # flips($b; $a): what differs, without the template or object, so one row covers every Policy
      # the same change reaches (the diff shows where). In parentheses: a comma binds tighter than a pipe.
      def flips($b; $a):
        (("remediationAction", "disabled") as $f | select($b[$f] != $a[$f])
          | "`\($f)` \($b[$f] | val) → \($a[$f] | val)"),
        (($b.templates | keys[]) as $t
          | if ($a.templates | has($t) | not) then "policy template `\($t)` removed"
            else $b.templates[$t] as $x | $a.templates[$t] as $y
              | ((("remediationAction", "pruneObjectBehavior", "complianceType") as $f | select($x[$f] != $y[$f])
                  | "`\($f)` \($x[$f] | val) → \($y[$f] | val)"),
                 (($x.objects | keys[]) as $o | select($y.objects | has($o)) | select($x.objects[$o] != $y.objects[$o])
                  | "`complianceType` \($x.objects[$o] | val) → \($y.objects[$o] | val)"))
            end);
    {added: (if $defaults then [$am | keys[] | select($bm[.] == null)] else [] end),
     removed: [$bm | keys[] | select($am[.] == null)],
     changed: [$am | keys[] | select($bm[.] != null and $bm[.] != $am[.])],
     enforcement: [$bm | to_entries[] | select((.key | startswith("Policy/")) and $am[.key] != null)
       | .key as $k | flips(.value | enforcement; $am[$k] | enforcement) | {k: ($k | s), c: .}]}'
}
gd() { git -c core.quotePath=false diff --no-index --no-color --no-ext-diff --no-prefix "$@" || [ $? = 1 ]; }
# names <json list>: up to five `Kind/entry`
names() { jq -r 'map(gsub("[`|<\n]"; "")) | (.[:5] | map("`" + . + "`") | join(", ")) + (if length > 5 then " and \(length - 5) more" else "" end)' <<< "$1"; }

declare -A sample=()
for el in "${render[@]}"; do
  failed=() hits=() all='{"added":[],"removed":[],"changed":[],"enforcement":[]}'
  for pass in "${passes[@]}"; do
    n=0
    while read -r f; do
      [ -n "$f" ] || continue
      rel=${f#base/"$pass"/} at=$(dirname "$rel") a=head/$pass/${f#base/"$pass"/}
      ! grep -q '^# RENDER000' "$f" || continue
      if [ -f "$a" ] && grep -q '^# RENDER000' "$a"; then failed+=("$pass $at"); continue; fi
      ! cmp -s "$f" "$a" || continue
      d=$(compare "$f" "$a" "$el-$(basename "$at")" "$([ "$pass" = defaults ] && echo true || echo false)")
      [ "$(jq '[.[][]] | length' <<< "$d")" -gt 0 ] || continue
      n=$((n + 1))
      all=$(jq -c --argjson d "$d" 'with_entries(.value += $d[.key] | .value |= unique)' <<< "$all")
      gd "$f" "$a" >> "$work/stable-gate.patch"
      if [ -z "${sample[$el]:-}" ]; then
        sample[$el]="$el: $pass, $at"
        gd "$f" "$a" | sed -E '/^(diff --git|index|new file|deleted file) /d' > "$work/details/$el"
      fi
    done < <(find "base/$pass" -name "$el.yaml" 2>/dev/null | sort)
    [ "$n" = 0 ] || hits+=("$pass: $n")
  done
  [ ${#failed[@]} = 0 ] ||
    breaks+=("$(row "$el" "render fails for unchanged values: $(printf '%s, ' "${failed[@]:0:5}" | sed 's/, $//')")")
  removed=$(jq -c .removed <<< "$all") changed=$(jq -c .changed <<< "$all") added=$(jq -c .added <<< "$all")
  [ "$removed" = '[]' ] || breaks+=("$(row "$el" "no longer renders for unchanged values: $(names "$removed")")")
  while read -r c; do
    [ -z "$c" ] || breaks+=("$(row "$el" "renders $(jq -r .c <<< "$c") in $(names "$(jq -c .k <<< "$c")")")")
  done < <(jq -c '.enforcement | group_by(.c)[] | {c: .[0].c, k: [.[].k]}' <<< "$all")
  what=()
  [ "$changed" = '[]' ] || what+=("changed: $(names "$changed")")
  [ "$added" = '[]' ] || what+=("added: $(names "$added")")
  [ ${#what[@]} = 0 ] || outputs+=("| \`$el\` | $(printf '%s<br>' "${hits[@]}" | sed 's/<br>$//') | $(printf '%s<br>' "${what[@]}" | sed 's/<br>$//') |")
done
cd "$work"

# table <heading> <instruction> <columns> <rows...>
table() {
  local heading=$1 text=$2 columns=$3
  shift 3
  [ $# -gt 0 ] || return 0
  printf '\n#### %s\n\n%s\n\n%s\n%s\n' "$heading" "$text" "$columns" "$(sed -E 's/[^|]+/---/g' <<< "$columns")"
  printf '%s\n' "$@"
}
found=false
[ $((${#breaks[@]} + ${#outputs[@]} + ${#promoted[@]})) = 0 ] || found=true
if [ "$found" = true ]; then
  {
    echo "### Stable elements: maintainer sign-off required"
    echo
    echo "Compared with the merge base, \`${base:0:7}\`. Both sides render with its test values."
    table Breaking "A real break merges only with \`!\` in the title, which releases a major. Sign off without \`!\` only for a false positive, or a security fix titled \`fix(<element>): breaking:\`." \
      "| Element | Finding |" ${breaks[@]+"${breaks[@]}"}
    table "Output changed for unchanged values" "Sign off only for a named bug fix, an operator channel or version move, or a new entry that ships off or inform-only. Anything else is breaking: retitle with \`!\`." \
      "| Element | Renders | Objects |" ${outputs[@]+"${outputs[@]}"}
    table Promoted "From the next release these elements' keys, Policy names and defaults follow the stable rules." \
      "| Element | Finding |" ${promoted[@]+"${promoted[@]}"}
    echo
  } > body.md
  # One collapsed diff per element, cut to stay under GitHub's 65536-character comment limit.
  for el in "${!sample[@]}"; do
    room=$((60000 - $(wc -c < body.md) - 600))
    [ "$room" -gt 2000 ] || break
    {
      printf '<details><summary><code>%s</code></summary>\n\n````diff\n' "${sample[$el]}"
      if [ "$(wc -c < "details/$el")" -le "$room" ]; then cat "details/$el"; else head -c "$room" "details/$el" | sed '$d'; echo '... cut, see the full patch'; fi
      printf '````\n\n</details>\n\n'
    } >> body.md
  done
  echo "Sign off on the [run](${RUN_URL:-}): **Review deployments**, then \`stable-change\`. Maintainers only, never the PR author. A new push needs a new sign-off." >> body.md
else
  echo "No stable element changes." > body.md
fi

cat body.md >> "$GITHUB_STEP_SUMMARY"
echo "found=$found" >> "$GITHUB_OUTPUT"
eof=EOF_$(openssl rand -hex 16) # the body holds PR content: no guessable delimiter
{ echo "body<<$eof"; cat body.md; echo "$eof"; } >> "$GITHUB_OUTPUT"
