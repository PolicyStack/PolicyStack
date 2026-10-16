#!/usr/bin/env bash
# Creates stack/<name> from tools/sample-element/, renaming my-element and myElement throughout.
# Usage: tools/create-element.sh [name] [description]
set -euo pipefail
cd "$(dirname "$0")/.."

name=${1:-} desc=${2:-}
[ -n "$name" ] || read -rp "Element name (lowercase kebab-case, short): " name
[[ $name =~ ^[a-z][a-z0-9]*(-[a-z0-9]+)*$ ]] || { echo "error: '$name' is not lowercase kebab-case" >&2; exit 1; }
[ -n "$desc" ] || read -rp "Description: " desc
dest=stack/$name
[ ! -e "$dest" ] || { echo "error: $dest already exists" >&2; exit 1; }

# kebab-case to camelCase (cert-manager -> certManager). Keep in sync with chart.CamelFromKebab.
camel=$(awk -F- '{printf "%s", $1; for (i = 2; i <= NF; i++) printf "%s", toupper(substr($i, 1, 1)) substr($i, 2)}' <<< "$name")

mkdir "$dest"
cp -r tools/sample-element/{.gitignore,Chart.yaml,values.yaml,templates,converters} "$dest"/
for f in "$dest"/converters/*my-element*; do mv "$f" "${f//my-element/$name}"; done
find "$dest" -type f -exec sed -i -e "s/\bmy-element\b/$name/g" -e "s/\bmyElement\b/$camel/g" {} +

# set_line <file> <prefix> <line>: replaces the line starting with prefix. ENVIRON keeps line literal.
set_line() { L=$3 awk -v p="$2" 'index($0, p) == 1 {$0 = ENVIRON["L"]} 1' "$1" > "$1.tmp" && mv "$1.tmp" "$1"; }
d=${desc//\\/\\\\} d=${d//\"/\\\"}
set_line "$dest/Chart.yaml" "description: " "description: \"$d\""
set_line "$dest/values.yaml" "  # @description: Example element" "  # @description: $desc"

echo "Created $dest, values key stack.$camel. Finish it as in docs/elements.md#creating-an-element."
