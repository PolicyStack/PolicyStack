#!/usr/bin/env bash
# Creates stack/<name> from tools/sample-element/, renaming my-element and myElement throughout.
# Usage: tools/create-element.sh [name] [description]
set -euo pipefail
cd "$(dirname "$0")/.."

die() { echo "error: $*" >&2; exit 1; }
name=${1:-} desc=${2:-}
[ -n "$name" ] || read -rp "Element name (lowercase kebab-case, short): " name
[[ $name =~ ^[a-z][a-z0-9]*(-[a-z0-9]+)*$ ]] || die "'$name' is not lowercase kebab-case"
# POLICY001: pstack.install-<name>-<30-character cluster> must fit 63 characters.
((${#name} <= 17)) || die "'$name' is ${#name} characters, the limit is 17 (https://policystk.io/concepts/policies/#naming-limits)"
# POLICY091: release <element>-<cluster> must not be ambiguous; lifecycle is reserved.
for e in stack/*/ lifecycle/; do
  e=${e%/} e=${e##*/}
  [[ $name == "$e" || $name == "$e"-* || $e == "$name"-* ]] && die "'$name' collides with element '$e' (POLICY091)"
done
[ -n "$desc" ] || read -rp "Description: " desc
dest=stack/$name

# kebab-case to camelCase (cert-manager -> certManager). Keep in sync with chart.CamelFromKebab.
camel=$(awk -F- '{printf "%s", $1; for (i = 2; i <= NF; i++) printf "%s", toupper(substr($i, 1, 1)) substr($i, 2)}' <<< "$name")

mkdir "$dest"
cp -r tools/sample-element/{.gitignore,Chart.yaml,values.yaml,templates,converters} "$dest"/
for f in "$dest"/converters/*/*my-element*; do b=${f##*/}; mv "$f" "${f%/*}/${b//my-element/$name}"; done
find "$dest" -type f -exec sed -i -e "s/\bmy-element\b/$name/g" -e "s/\bmyElement\b/$camel/g" {} +

# set_line <file> <prefix> <line>: replaces the line starting with prefix. ENVIRON keeps line literal.
set_line() { L=$3 awk -v p="$2" 'index($0, p) == 1 {$0 = ENVIRON["L"]} 1' "$1" > "$1.tmp" && mv "$1.tmp" "$1"; }
d=${desc//\\/\\\\} d=${d//\"/\\\"}
set_line "$dest/Chart.yaml" "description: " "description: \"$d\""

# Upstream only: copier's answers template, so adopters can copy the element. In a copy (no
# .release-please/) the element stays local, without one.
[ ! -d .release-please ] || cp '{{_copier_conf.answers_file}}.jinja' "$dest"/

python3 tools/doc-generator.py --element "$name" >/dev/null ||
  die "README not generated: pip install -r tools/requirements.txt, then python3 tools/doc-generator.py --element $name"
echo "Created $dest, values key stack.$camel. Next steps: https://policystk.io/develop/elements/#workflow"
