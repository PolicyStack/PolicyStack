#!/usr/bin/env bash
# Renders every fleet file whose output can change, before and after the PR, and writes a comment
# body summarising what changes per cluster. Run from the PR merge commit (HEAD^1 is the base tip).
#
# Env: DEFAULT_BRANCH, RUN_URL, RUNNER_TEMP, GITHUB_OUTPUT, GITHUB_STEP_SUMMARY.
# Writes $RUNNER_TEMP/render-diff/{body.md,render-diff.patch} and sets changed=true|false.
set -euo pipefail

base=$(git rev-parse HEAD^1) merge=$(git rev-parse HEAD)
bin=$PWD/tools/validator/bin/policystack-validator
selector=$PWD/tools/validator/testdata/baseline.yaml # stands in for the selector the ApplicationSet injects
work=$RUNNER_TEMP/render-diff
rm -rf "$work" && git worktree prune && mkdir -p "$work/before" "$work/after" "$work/details"

# fleet <commit>: fleet file paths at commit
fleet() { git ls-tree --name-only "$1" -- fleet/ fleet/hubs/ | grep '\.yaml$' || true; }
# resolve <commit> <revision>: the commit Argo CD renders for revision when the default branch is at
# commit, in the order validate.yml's pin check uses: tag, branch, SHA.
resolve() {
  case $2 in "") return 1 ;; HEAD | "$DEFAULT_BRANCH") echo "$1"; return ;; esac
  git rev-parse -q --verify "refs/tags/$2^{commit}" ||
    git rev-parse -q --verify "refs/remotes/origin/$2^{commit}" ||
    { [[ $2 =~ ^[0-9a-f]{5,40}$ ]] && git rev-parse -q --verify "$2^{commit}"; }
}
# finish <changed> [<summary when unchanged>]
finish() {
  echo "changed=$1" >> "$GITHUB_OUTPUT"
  if [ "$1" = false ]; then echo "$2" >> "$GITHUB_STEP_SUMMARY"; exit 0; fi
  cat "$work/body.md" >> "$GITHUB_STEP_SUMMARY"
  eof=EOF_$(openssl rand -hex 16) # the body holds PR content: no guessable delimiter
  { echo "body<<$eof"; cat "$work/body.md"; echo "$eof"; } >> "$GITHUB_OUTPUT"
  exit 0
}

# Per <side>/<fleet file without fleet/ and .yaml>: blob, revision, the commit it renders (- if the
# revision does not resolve) and, when the validator cannot load that commit, its exit code.
declare -A blob=() rev=() sha=() broken=()
for side in before after; do
  at=$base
  [ "$side" = before ] || at=$merge
  for f in $(fleet "$at"); do
    k=$side/${f#fleet/} k=${k%.yaml}
    blob[$k]=$(git rev-parse "$at:$f")
    rev[$k]=$(git cat-file blob "${blob[$k]}" | yq '.revision | select(tag == "!!str")' - 2>/dev/null) || rev[$k]=
    sha[$k]=$(resolve "$at" "${rev[$k]}") || sha[$k]=-
  done
done

# Clusters whose file or rendered commit differs. One fixtures dir per side and commit.
declare -A clusters=()
for k in "${!blob[@]}"; do
  c=${k#*/} side=${k%%/*} other=after
  [ "$side" = before ] || other=before
  [ "${sha[$k]} ${blob[$k]}" != "${sha[$other/$c]:-} ${blob[$other/$c]:-}" ] || continue
  clusters[$c]=1
  [ "${sha[$k]}" != - ] || continue
  mkdir -p "$work/fleet/$side/${sha[$k]}/hubs"
  git cat-file blob "${blob[$k]}" > "$work/fleet/$side/${sha[$k]}/$c.yaml"
done
[ ${#clusters[@]} -gt 0 ] || finish false "No cluster's rendered Policies change."

# One worktree per commit, shared by both sides.
for group in "$work"/fleet/*/*; do
  [ -d "$group" ] || continue
  side=$(basename "$(dirname "$group")") c=$(basename "$group")
  [ -d "$work/tree/$c" ] || git worktree add -q --detach "$work/tree/$c" "$c"
  echo "::group::Render $side at ${c:0:7}"
  # Exit 1 is a failed render, written into the render as # RENDER000 comments. Above 1 the
  # validator could not load the tree at all.
  rc=0
  "$bin" --repo-root "$work/tree/$c" --fixtures-dir "$group" --extra-values "$selector" \
    --only RENDER000 --render-dir "$work/$side" || rc=$?
  echo "::endgroup::"
  if [ "$rc" -gt 1 ]; then
    for f in "$group"/*.yaml "$group"/hubs/*.yaml; do
      rel=${f#"$group"/}
      [ ! -f "$f" ] || broken[$side/${rel%.yaml}]=$rc
    done
  fi
done

cd "$work"
# gd <before> <after>: the patch, with paths relative to the side
gd() {
  { git -c core.quotePath=false diff --no-index --no-renames --no-color --no-ext-diff --no-prefix "$@" || [ $? = 1 ]; } |
    sed -E -e '/^diff --git /s# (before|after)/# #g' -e 's#^(---|\+\+\+) (before|after)/#\1 #'
}
# objects <file>: one {k: kind/name, v: object} per line
objects() { [ ! -f "$1" ] || yq -o=json -I=0 'select(.kind != null) | {"k": .kind + "/" + .metadata.name, "v": .}' "$1"; }
# counts <before> <after>: e.g. "1 Policy changed, 2 Placements added", objects keyed by kind and name
counts() {
  jq -rn --slurpfile b <(objects "$1") --slurpfile a <(objects "$2") '
    def byKey: map({(.k): .v}) | add // {};
    def plural: if endswith("y") then .[:-1] + "ies" else . + "s" end;
    def kinds($verb): group_by(split("/")[0])
      | map((.[0] | split("/")[0]) as $kind | "\(length) \(if length > 1 then ($kind | plural) else $kind end) \($verb)");
    ($b | byKey) as $b | ($a | byKey) as $a |
    ([$a | keys[] | select($b[.] == null)] | kinds("added"))
      + ([$b | keys[] | select($a[.] == null)] | kinds("removed"))
      + ([$a | keys[] | select($b[.] != null and $b[.] != $a[.])] | kinds("changed"))
    | join(", ")'
}
# total <dir>: "6 elements, 21 objects"
total() { echo "$(find "$1" -name '*.yaml' | wc -l) elements, $(cat /dev/null "$1"/*.yaml 2>/dev/null | yq -N 'select(.kind != null) | .kind' | wc -l) objects"; }
# name <cluster> <open> <close>: the cluster's name in code delimiters, hubs marked
name() { case $1 in hubs/*) echo "$2${1#hubs/}$3 (hub)" ;; *) echo "$2$1$3" ;; esac; }
# r <side/cluster>: the revision for markdown, without characters that break a table or code span
r() {
  local s=${rev[$1]:-}
  [ -n "$s" ] || { echo "(none)"; return; }
  s=${s//|/"\\|"} s=${s//\`/} s=${s//</"&lt;"}
  echo "\`${s//$'\n'/ }\`"
}
# file <path>: path, or /dev/null when the element does not render on that side
file() { if [ -f "$1" ]; then echo "$1"; else echo /dev/null; fi; }

gd before after > render-diff.patch
rows=() warnings=()
declare -A state=()
for c in $(printf '%s\n' "${!clusters[@]}" | sort); do
  b=before/$c a=after/$c d=details/${c//\//_}
  mkdir -p "$b" "$a"
  # A side is absent, bad (does not resolve or cannot be loaded) or rendered.
  for side in before after; do
    if [ -z "${blob[$side/$c]:-}" ]; then state[$side]=absent
    elif [ "${sha[$side/$c]}" = - ] || [ -n "${broken[$side/$c]:-}" ]; then state[$side]=bad
    else state[$side]=ok; fi
  done
  if [ "${state[after]}" = bad ]; then
    if [ "${sha[$a]}" = - ]; then
      warnings+=("$(name "$c" '`' '`') pins $(r "$a"), which is not a tag, branch or commit. Merging stops Argo CD from generating Applications for every cluster until it is fixed.")
    else
      warnings+=("$(name "$c" '`' '`'): the validator cannot load $(r "$a") (exit ${broken[$a]}), see the job log.")
    fi
    continue
  fi
  changes=()
  case ${state[before]}/${state[after]} in
    absent/ok | bad/ok) revision="new at $(r "$a")" changes=("$(total "$a") added") ;;
    ok/absent) revision="removed, was $(r "$b")" changes=("$(total "$b") removed") ;;
    bad/absent) continue ;;
    ok/ok)
      revision=$(r "$a")
      [ "${rev[$b]}" = "${rev[$a]}" ] || revision="$(r "$b") → $(r "$a")"
      for el in $(ls "$b" "$a" | grep '\.yaml$' | sort -u); do
        ! cmp -s "$b/$el" "$a/$el" || continue
        if grep -q '^# RENDER000' "$a/$el" 2>/dev/null; then
          changes+=("${el%.yaml}: **render failed**")
          printf '%s: render failed\n````\n%s\n````\n\n' "${el%.yaml}" "$(sed 's/^# //' "$a/$el")" >> "$d"
          continue
        fi
        if grep -q '^# RENDER000' "$b/$el" 2>/dev/null; then
          changes+=("${el%.yaml}: render fixed")
          continue
        fi
        n=$(counts "$b/$el" "$a/$el")
        [ -n "$n" ] || continue
        changes+=("${el%.yaml}: $n")
        { echo '````diff'; gd "$(file "$b/$el")" "$(file "$a/$el")" | sed -E '/^(diff --git|index|new file|deleted file) /d'; printf '````\n\n'; } >> "$d"
      done
      ;;
  esac
  [ ${#changes[@]} -gt 0 ] || continue
  rows+=("| $(name "$c" '`' '`') | $revision | $(printf '%s<br>' "${changes[@]}" | sed 's/<br>$//') |")
done
[ $((${#rows[@]} + ${#warnings[@]})) -gt 0 ] || finish false "No cluster's rendered Policies change."

{
  if [ ${#rows[@]} -gt 0 ]; then echo "### Fleet render: ${#rows[@]} cluster(s) change"; else echo "### Fleet render: no cluster changes"; fi
  echo
  for w in "${warnings[@]}"; do printf '> [!WARNING]\n> %s\n\n' "$w"; done
  if [ ${#rows[@]} -gt 0 ]; then
    echo "| Cluster | Revision | Changes |"
    echo "|---|---|---|"
    printf '%s\n' "${rows[@]}"
    echo
  fi
} > body.md
# One collapsed block per cluster, cut to stay under GitHub's 65536-character comment limit.
budget=60000 left=()
for c in $(printf '%s\n' "${!clusters[@]}" | sort); do
  d=details/${c//\//_}
  [ -s "$d" ] || continue
  room=$((budget - $(wc -c < body.md) - 200))
  if [ "$room" -lt 2000 ]; then left+=("$c"); continue; fi
  {
    printf '<details><summary>%s</summary>\n\n' "$(name "$c" '<code>' '</code>')"
    if [ "$(wc -c < "$d")" -le "$room" ]; then
      cat "$d"
    else
      kept=$(head -c "$room" "$d" | sed '$d')
      printf '%s\n... cut, see the full patch\n' "$kept"
      # Close the fence the cut left open.
      [ $(($(grep -c '^````' <<< "$kept") % 2)) = 0 ] || printf '````\n'
      echo
    fi
    printf '</details>\n\n'
  } >> body.md
done
[ ${#left[@]} -eq 0 ] || echo "Left out for length: ${left[*]}." >> body.md
echo "Rendered at each fleet file's \`revision\`, before → after this PR. [Full patch]($RUN_URL#artifacts)" >> body.md
finish true
